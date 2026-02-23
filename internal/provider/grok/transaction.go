// Package grok implements the Grok (X.com) web API provider.
//
// transaction.go contains the x-client-transaction-id generator that
// is required for write endpoints (add_response). It fetches the X
// homepage, extracts cryptographic material from the ondemand.s JS
// and SVG loading animations, then produces a signed transaction ID.
package grok

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	defaultTransactionKeyword = "obfiowerehiring"
	transactionCacheTTL       = 1 * time.Hour
)

var (
	reOnDemand    = regexp.MustCompile(`['"]ondemand\.s['"]:\s*['"](\w*)['"]`)
	reIndices     = regexp.MustCompile(`\(\w\[(\d{1,2})\],\s*16\)`)
	reMetaVerif   = regexp.MustCompile(`<meta\b[^>]*\bname=['"]twitter-site-verification['"][^>]*\bcontent=['"]([^'"]+)['"][^>]*>`)
	reMetaVerif2  = regexp.MustCompile(`<meta\b[^>]*\bcontent=['"]([^'"]+)['"][^>]*\bname=['"]twitter-site-verification['"][^>]*>`)
	reLoadingAnim = regexp.MustCompile(`(?is)<svg\b[^>]*\bid=['"]loading-x-anim[^'"]*['"][^>]*>(.*?)</svg>`)
	rePathD       = regexp.MustCompile(`(?i)<path\b[^>]*\bd=['"]([^'"]+)['"][^>]*>`)
)

// transactionGenerator produces x-client-transaction-id values for
// Grok write endpoints. It caches the homepage-derived crypto material
// for up to 1 hour before refreshing.
type transactionGenerator struct {
	mu sync.Mutex

	homePageHTML      string
	defaultRowIndex   int
	defaultKeyIndices []int
	key               string
	keyBytes          []byte
	animationKey      string
	initialized       bool
	cachedAt          time.Time

	userAgent string
	logf      func(string, ...any)
}

func newTransactionGenerator(userAgent string, logf func(string, ...any)) *transactionGenerator {
	return &transactionGenerator{
		userAgent: userAgent,
		logf:      logf,
	}
}

// generateID produces a transaction ID for the given HTTP method + path.
func (g *transactionGenerator) generateID(method, path string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if err := g.ensureInitialized(); err != nil {
		return "", err
	}

	now := int64(time.Now().Unix()) - 1682924400
	timeBytes := []byte{
		byte(now & 0xff),
		byte((now >> 8) & 0xff),
		byte((now >> 16) & 0xff),
		byte((now >> 24) & 0xff),
	}

	data := fmt.Sprintf("%s!%s!%d%s%s", method, path, now, defaultTransactionKeyword, g.animationKey)
	hash := sha256.Sum256([]byte(data))

	randomByte := make([]byte, 1)
	rand.Read(randomByte)
	rnd := randomByte[0]

	bytesArr := make([]byte, 0, len(g.keyBytes)+4+16+1)
	bytesArr = append(bytesArr, g.keyBytes...)
	bytesArr = append(bytesArr, timeBytes...)
	bytesArr = append(bytesArr, hash[:16]...)
	bytesArr = append(bytesArr, 3) // ADDITIONAL_RANDOM_NUMBER

	out := make([]byte, 1+len(bytesArr))
	out[0] = rnd
	for i, b := range bytesArr {
		out[i+1] = b ^ rnd
	}

	encoded := base64.StdEncoding.EncodeToString(out)
	return strings.TrimRight(encoded, "="), nil
}

func (g *transactionGenerator) ensureInitialized() error {
	if g.initialized && time.Since(g.cachedAt) < transactionCacheTTL {
		return nil
	}

	html, err := g.fetchHomepageHTML()
	if err != nil {
		return fmt.Errorf("fetching X homepage: %w", err)
	}
	g.homePageHTML = html
	g.cachedAt = time.Now()

	rowIndex, keyIndices, err := g.getIndices()
	if err != nil {
		return fmt.Errorf("getting transaction indices: %w", err)
	}
	g.defaultRowIndex = rowIndex
	g.defaultKeyIndices = keyIndices

	g.key = g.getKey()
	g.keyBytes = g.getKeyBytes(g.key)
	g.animationKey = g.getAnimationKey(g.keyBytes)
	g.initialized = true
	return nil
}

func (g *transactionGenerator) fetchHomepageHTML() (string, error) {
	req, err := http.NewRequest("GET", "https://x.com", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("user-agent", g.userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("X homepage returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (g *transactionGenerator) getIndices() (int, []int, error) {
	match := reOnDemand.FindStringSubmatch(g.homePageHTML)
	if len(match) < 2 || match[1] == "" {
		return 0, nil, errors.New("could not find ondemand.s hash on homepage")
	}

	onDemandURL := fmt.Sprintf("https://abs.twimg.com/responsive-web/client-web/ondemand.s.%sa.js", match[1])
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(onDemandURL)
	if err != nil {
		return 0, nil, fmt.Errorf("fetching ondemand file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, nil, fmt.Errorf("ondemand file returned HTTP %d", resp.StatusCode)
	}
	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	matches := reIndices.FindAllStringSubmatch(string(text), -1)
	if len(matches) < 2 {
		return 0, nil, errors.New("couldn't get KEY_BYTE indices")
	}

	indices := make([]int, 0, len(matches))
	for _, m := range matches {
		val, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, nil, fmt.Errorf("invalid index: %w", err)
		}
		indices = append(indices, val)
	}

	return indices[0], indices[1:], nil
}

func (g *transactionGenerator) getKey() string {
	match := reMetaVerif.FindStringSubmatch(g.homePageHTML)
	if len(match) >= 2 && match[1] != "" {
		return match[1]
	}
	match = reMetaVerif2.FindStringSubmatch(g.homePageHTML)
	if len(match) >= 2 && match[1] != "" {
		return match[1]
	}
	return ""
}

func (g *transactionGenerator) getKeyBytes(key string) []byte {
	padded := key
	if rem := len(padded) % 4; rem != 0 {
		padded += strings.Repeat("=", 4-rem)
	}
	decoded, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		return []byte(key)
	}
	return decoded
}

func (g *transactionGenerator) getFramesD() []string {
	svgMatches := reLoadingAnim.FindAllStringSubmatch(g.homePageHTML, -1)
	var frames []string
	for _, svgMatch := range svgMatches {
		if len(svgMatch) < 2 {
			continue
		}
		inner := svgMatch[1]
		pathMatches := rePathD.FindAllStringSubmatch(inner, -1)
		var ds []string
		for _, pm := range pathMatches {
			if len(pm) >= 2 {
				ds = append(ds, pm[1])
			}
		}
		if len(ds) > 1 {
			frames = append(frames, ds[1])
		} else if len(ds) > 0 {
			frames = append(frames, ds[0])
		}
	}
	return frames
}

func (g *transactionGenerator) get2dArray(keyBytes []byte) [][]int {
	frames := g.getFramesD()
	if len(frames) == 0 {
		return [][]int{{}}
	}
	frameIndex := int(keyBytes[5]) % 4
	dAttr := frames[frameIndex%len(frames)]

	if len(dAttr) <= 9 {
		return [][]int{}
	}
	items := strings.Split(dAttr[9:], "C")
	result := make([][]int, len(items))
	for i, item := range items {
		cleaned := strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return ' '
		}, item)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			result[i] = []int{}
			continue
		}
		parts := strings.Fields(cleaned)
		row := make([]int, len(parts))
		for j, p := range parts {
			row[j], _ = strconv.Atoi(p)
		}
		result[i] = row
	}
	return result
}

func (g *transactionGenerator) solve(value, minVal, maxVal float64, rounding bool) float64 {
	result := value*(maxVal-minVal)/255 + minVal
	if rounding {
		return math.Floor(result)
	}
	return math.Round(result*100) / 100
}

func (g *transactionGenerator) animate(frames []int, targetTime float64) string {
	get := func(i int) float64 {
		if i < len(frames) {
			return float64(frames[i])
		}
		return 0
	}

	fromColor := []float64{get(0), get(1), get(2), 1}
	toColor := []float64{get(3), get(4), get(5), 1}
	fromRotation := []float64{0.0}
	toRotation := []float64{g.solve(get(6), 60.0, 360.0, true)}

	remaining := frames[7:]
	curves := make([]float64, len(remaining))
	for i, item := range remaining {
		curves[i] = g.solve(float64(item), isOdd(i), 1.0, false)
	}

	var c4 [4]float64
	for i := 0; i < 4 && i < len(curves); i++ {
		c4[i] = curves[i]
	}
	cubic := &cubicBezier{curves: c4}
	val := cubic.getValue(targetTime)

	color := interpolate(fromColor, toColor, val)
	for i := range color {
		if color[i] < 0 {
			color[i] = 0
		}
	}
	rotation := interpolate(fromRotation, toRotation, val)
	matrix := convertRotationToMatrix(rotation[0])

	var strArr []string
	for _, v := range color[:3] {
		strArr = append(strArr, fmt.Sprintf("%x", int(math.Round(v))))
	}
	for _, v := range matrix {
		rounded := math.Round(v*100) / 100
		if rounded < 0 {
			rounded = -rounded
		}
		hexVal := floatToHex(rounded)
		if strings.HasPrefix(hexVal, ".") {
			hexVal = "0" + hexVal
		}
		strArr = append(strArr, strings.ToLower(hexVal))
	}
	strArr = append(strArr, "0", "0")

	joined := strings.Join(strArr, "")
	joined = strings.ReplaceAll(joined, ".", "")
	joined = strings.ReplaceAll(joined, "-", "")
	return joined
}

func (g *transactionGenerator) getAnimationKey(keyBytes []byte) string {
	const totalTime = 4096

	if len(keyBytes) <= g.defaultRowIndex || len(keyBytes) <= 5 {
		return ""
	}

	rowIndex := int(keyBytes[g.defaultRowIndex]) % 16
	frameTime := 1
	for _, idx := range g.defaultKeyIndices {
		if idx < len(keyBytes) {
			frameTime *= int(keyBytes[idx]) % 16
		}
	}
	frameTime = (frameTime / 10) * 10

	arr := g.get2dArray(keyBytes)
	if rowIndex >= len(arr) {
		return ""
	}
	row := arr[rowIndex]

	targetTime := float64(frameTime) / float64(totalTime)
	return g.animate(row, targetTime)
}

// --- math helpers ---

type cubicBezier struct {
	curves [4]float64
}

func (c *cubicBezier) getValue(t float64) float64 {
	if t <= 0.0 {
		var startGradient float64
		if c.curves[0] > 0.0 {
			startGradient = c.curves[1] / c.curves[0]
		} else if c.curves[1] == 0.0 && c.curves[2] > 0.0 {
			startGradient = c.curves[3] / c.curves[2]
		}
		return startGradient * t
	}
	if t >= 1.0 {
		var endGradient float64
		if c.curves[2] < 1.0 {
			endGradient = (c.curves[3] - 1.0) / (c.curves[2] - 1.0)
		} else if c.curves[2] == 1.0 && c.curves[0] < 1.0 {
			endGradient = (c.curves[1] - 1.0) / (c.curves[0] - 1.0)
		}
		return 1.0 + endGradient*(t-1.0)
	}

	start := 0.0
	mid := 0.0
	end := 1.0
	for start < end {
		mid = (start + end) / 2
		xEst := cubicCalc(c.curves[0], c.curves[2], mid)
		if math.Abs(t-xEst) < 0.00001 {
			return cubicCalc(c.curves[1], c.curves[3], mid)
		}
		if xEst < t {
			start = mid
		} else {
			end = mid
		}
	}
	return cubicCalc(c.curves[1], c.curves[3], mid)
}

func cubicCalc(a, b, m float64) float64 {
	return 3.0*a*(1-m)*(1-m)*m + 3.0*b*(1-m)*m*m + m*m*m
}

func interpolate(from, to []float64, f float64) []float64 {
	out := make([]float64, len(from))
	for i := range from {
		out[i] = from[i]*(1-f) + to[i]*f
	}
	return out
}

func convertRotationToMatrix(rotation float64) []float64 {
	rad := rotation * math.Pi / 180
	return []float64{math.Cos(rad), -math.Sin(rad), math.Sin(rad), math.Cos(rad)}
}

func floatToHex(x float64) string {
	if x == 0 {
		return "0"
	}
	var result []byte
	intPart := int(math.Floor(x))
	fraction := x - float64(intPart)

	if intPart > 0 {
		n := intPart
		for n > 0 {
			remainder := n % 16
			if remainder > 9 {
				result = append([]byte{byte(remainder + 55)}, result...)
			} else {
				result = append([]byte{byte('0' + remainder)}, result...)
			}
			n /= 16
		}
	}

	if fraction == 0 {
		if len(result) == 0 {
			return "0"
		}
		return string(result)
	}

	result = append(result, '.')
	for fraction > 0 {
		fraction *= 16
		integer := int(math.Floor(fraction))
		fraction -= float64(integer)
		if integer > 9 {
			result = append(result, byte(integer+55))
		} else {
			result = append(result, byte('0'+integer))
		}
	}
	return string(result)
}

func isOdd(num int) float64 {
	if num%2 != 0 {
		return -1.0
	}
	return 0.0
}

// generateRandomTransactionID returns a random hex string for read endpoints.
func generateRandomTransactionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

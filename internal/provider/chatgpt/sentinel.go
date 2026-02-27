// Package chatgpt — sentinel.go implements OpenAI's sentinel proof-of-work
// challenge used as an anti-bot gate before the conversation endpoint.
//
// Flow:
//  1. Build a browser-fingerprint config array.
//  2. POST /backend-api/sentinel/chat-requirements with {"p": <token>}
//  3. Response contains a chat_token and optionally a proofofwork challenge.
//  4. If required, brute-force a SHA3-512 nonce whose hex prefix ≤ difficulty.
//  5. Attach sentinel headers to the conversation request.
package chatgpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/kyupark/ask/internal/httpclient"
	"golang.org/x/crypto/sha3"
)

const (
	sentinelPath   = "/backend-api/sentinel/chat-requirements"
	maxIterations  = 1_000_000
	errorPrefix    = "gAAAAABwQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D"
	resultPrefix   = "gAAAAAB"
	timeLayout     = "Mon Jan 02 2006 15:04:05"
	defaultScript  = "https://cdn.oaistatic.com/_next/static/chunks/app/layout-BuaxVDeh.js"
	defaultDPL     = "4811fd1c94b550c8f03fcc863ee6c1a99940efc5"
	navigatorKey   = "updateAdInterestGroups\u2212function updateAdInterestGroups() { [native code] }"
	documentKey    = "location"
	windowKey      = "__NEXT_PRELOADREADY"
	defaultPerfVal = 885.6999999880791
)

var (
	cores   = []int{1, 2, 4, 8, 12, 16, 24}
	screens = []int{3000, 4000, 6000}
)

// sentinelResult holds everything needed to set sentinel headers on the
// conversation request.
type sentinelResult struct {
	ChatToken  string
	ProofToken string
}

// chatRequirementsReq is the POST body for the sentinel endpoint.
type chatRequirementsReq struct {
	P string `json:"p"`
}

// chatRequirementsResp is the response from the sentinel endpoint.
type chatRequirementsResp struct {
	Token       string `json:"token"`
	ProofOfWork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	ForceLogin bool `json:"force_login"`
}

// acquireSentinel performs the full sentinel handshake:
// fetch chat-requirements → solve PoW if needed → return tokens.
func (p *Provider) acquireSentinel(ctx context.Context, logf func(string, ...any)) (*sentinelResult, error) {
	config := buildConfig(p.userAgent)

	// Build a simple "p" value.  The referenced implementations send either
	// a static string or a light token; a random UUID-ish string works.
	pToken := "hello openai" + newUUID()

	reqBody, _ := json.Marshal(chatRequirementsReq{P: pToken})

	url := p.baseURL + sentinelPath
	logf("[chatgpt] POST %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("sentinel request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("OAI-Device-Id", p.deviceID)
	req.Header.Set("OAI-Language", "en-US")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")

	if p.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.accessToken)
	}
	p.setCookies(req)

	client := httpclient.New(p.timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sentinel request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("sentinel HTTP %d: %s", resp.StatusCode, string(body))
	}

	var cresp chatRequirementsResp
	if err := json.NewDecoder(resp.Body).Decode(&cresp); err != nil {
		return nil, fmt.Errorf("sentinel decode: %w", err)
	}

	if cresp.ForceLogin {
		return nil, fmt.Errorf("ChatGPT requires login — session may be expired")
	}

	if cresp.Token == "" {
		return nil, fmt.Errorf("sentinel returned empty chat token")
	}

	result := &sentinelResult{ChatToken: cresp.Token}

	// Solve proof-of-work if required.
	if cresp.ProofOfWork.Required {
		seed := cresp.ProofOfWork.Seed
		diff := cresp.ProofOfWork.Difficulty
		logf("[chatgpt] PoW required: seed=%s diff=%s", seed, diff)

		token, solved := solveProofOfWork(config, seed, diff)
		if !solved {
			logf("[chatgpt] PoW: fell back to error token after %d iterations", maxIterations)
		} else {
			logf("[chatgpt] PoW solved")
		}
		result.ProofToken = token
	}

	return result, nil
}

// buildConfig creates the browser-fingerprint config array that gets
// JSON-serialized and base64-encoded in the PoW loop.
func buildConfig(userAgent string) []interface{} {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	core := cores[rng.Intn(len(cores))]
	screen := screens[rng.Intn(len(screens))]

	return []interface{}{
		core + screen,     // 0: cores + screen resolution bucket
		getParseTime(),    // 1: formatted timestamp
		int64(4294705152), // 2: magic constant (WebGL renderer hash)
		0,                 // 3: iteration counter (mutated in loop)
		userAgent,         // 4: User-Agent
		defaultScript,     // 5: script source URL
		defaultDPL,        // 6: deployment hash
		"en-US",           // 7: language
		"en-US,en",        // 8: languages
		0,                 // 9: elapsed time in ms (mutated in loop)
		navigatorKey,      // 10: navigator property fingerprint
		documentKey,       // 11: document property key
		windowKey,         // 12: window property key
		defaultPerfVal,    // 13: performance timing value
	}
}

// getParseTime returns a timestamp string mimicking a US-timezone browser.
func getParseTime() string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	return now.Format(timeLayout) + " GMT-0800 (Pacific Time)"
}

// solveProofOfWork brute-forces a nonce such that
// SHA3-512(seed || base64(config_with_nonce)) has a hex prefix ≤ difficulty.
//
// Returns ("gAAAAAB" + base64_solution, true) on success, or a fallback
// error token on exhaustion.
func solveProofOfWork(config []interface{}, seed, diff string) (string, bool) {
	diffLen := len(diff) / 2 // difficulty is hex — compare raw bytes
	if diffLen == 0 {
		diffLen = 1
	}

	hasher := sha3.New512()
	seedBytes := []byte(seed)
	startTime := time.Now()

	for i := 0; i < maxIterations; i++ {
		config[3] = i
		config[9] = time.Since(startTime).Milliseconds()

		jsonData, _ := json.Marshal(config)
		b64 := base64.StdEncoding.EncodeToString(jsonData)

		hasher.Write(seedBytes)
		hasher.Write([]byte(b64))
		hash := hasher.Sum(nil)
		hasher.Reset()

		if hex.EncodeToString(hash[:diffLen]) <= diff {
			return resultPrefix + b64, true
		}
	}

	// Fallback: send an error token so the request at least proceeds.
	fallback := errorPrefix + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

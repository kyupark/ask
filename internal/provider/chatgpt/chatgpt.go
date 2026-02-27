// Package chatgpt implements the ChatGPT web API provider.
package chatgpt

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/kyupark/ask/internal/httpclient"
	"github.com/kyupark/ask/internal/provider"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://chatgpt.com"
	sessionPath      = "/api/auth/session"
	conversationPath = "/backend-api/conversation"

	cookieSessionToken = "__Secure-next-auth.session-token"
	cookieCfClearance  = "cf_clearance"
	cookiePUID         = "_puid"
	domainChatGPT      = "chatgpt.com"
	domainOpenAI       = "openai.com"
)

// --- Request/Response types ---

type author struct {
	Role string `json:"role"`
}

type content struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}

type messageMetadata struct {
	SerializationMetadata serializationMetadata `json:"serialization_metadata"`
}

type serializationMetadata struct {
	CustomSymbolOffsets []interface{} `json:"custom_symbol_offsets"`
}
type message struct {
	ID         string          `json:"id"`
	Author     author          `json:"author"`
	Content    content         `json:"content"`
	CreateTime float64         `json:"create_time"`
	Metadata   messageMetadata `json:"metadata"`
}
type conversationMode struct {
	Kind string `json:"kind"`
}

type clientContextualInfo struct {
	IsDarkMode      bool `json:"is_dark_mode"`
	TimeSinceLoaded int  `json:"time_since_loaded"`
	PageHeight      int  `json:"page_height"`
	PageWidth       int  `json:"page_width"`
	PixelRatio      int  `json:"pixel_ratio"`
	ScreenHeight    int  `json:"screen_height"`
	ScreenWidth     int  `json:"screen_width"`
}
type conversationRequest struct {
	Action                           string               `json:"action"`
	Messages                         []message            `json:"messages"`
	ParentMessageID                  string               `json:"parent_message_id"`
	Model                            string               `json:"model"`
	ConversationID                   string               `json:"conversation_id,omitempty"`
	TimezoneOffsetMin                int                  `json:"timezone_offset_min"`
	Timezone                         string               `json:"timezone"`
	HistoryAndTrainingDisabled       bool                 `json:"history_and_training_disabled,omitempty"`
	ConversationMode                 conversationMode     `json:"conversation_mode"`
	EnableMessageFollowups           bool                 `json:"enable_message_followups"`
	SystemHints                      []string             `json:"system_hints"`
	ThinkingEffort                   string               `json:"thinking_effort,omitempty"`
	SupportsBuffering                bool                 `json:"supports_buffering,omitempty"`
	SupportedEncodings               []string             `json:"supported_encodings,omitempty"`
	ClientContextualInfo             clientContextualInfo `json:"client_contextual_info"`
	ParagenCotSummaryDisplayOverride string               `json:"paragen_cot_summary_display_override"`
}

type responseMessage struct {
	ID      string  `json:"id"`
	Author  author  `json:"author"`
	Content content `json:"content"`
}

type conversationResponse struct {
	Message        *responseMessage `json:"message,omitempty"`
	ConversationID string           `json:"conversation_id"`
}

type sessionResponse struct {
	AccessToken string `json:"accessToken"`
}

// Provider implements the ChatGPT web API backend.
type Provider struct {
	thinkingEffort string
	baseURL        string
	model          string
	userAgent      string
	timeout        time.Duration
	sessionToken   string
	cfClearance    string
	puid           string
	deviceID       string
	// Cached auth state.
	accessToken string
	tokenExpiry time.Time
}

// New creates a ChatGPT provider.
func New(baseURL, model, userAgent string, timeout time.Duration) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if model == "" {
		model = "auto"
	}
	return &Provider{
		baseURL:   baseURL,
		model:     model,
		userAgent: userAgent,
		timeout:   timeout,
		deviceID:  newUUID(),
	}
}

func (p *Provider) Name() string { return "chatgpt" }

func (p *Provider) CookieSpecs() []provider.CookieSpec {
	return []provider.CookieSpec{
		{Domain: domainChatGPT, Names: []string{cookieSessionToken, cookieCfClearance, cookiePUID}},
		{Domain: domainOpenAI, Names: []string{cookieSessionToken, cookieCfClearance, cookiePUID}},
	}
}

func (p *Provider) SetCookies(cookies map[string]string) {
	if v := cookies[cookieSessionToken]; v != "" {
		p.sessionToken = v
	}
	if v := cookies[cookieCfClearance]; v != "" {
		p.cfClearance = v
	}
	if v := cookies[cookiePUID]; v != "" {
		p.puid = v
	}
}

// SetThinkingEffort sets the thinking effort level (none, low, medium, high, xhigh).
func (p *Provider) SetThinkingEffort(effort string) { p.thinkingEffort = effort }

func (p *Provider) Ask(ctx context.Context, query string, opts provider.AskOptions) error {
	if p.sessionToken == "" {
		return fmt.Errorf("no session cookie — log in to chatgpt.com in your browser")
	}
	logf := opts.LogFunc
	if logf == nil {
		logf = func(string, ...any) {}
	}
	// Refresh access token if needed.
	token, err := p.getAccessToken(ctx, logf)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	// Acquire sentinel tokens (chat-requirements + PoW).
	sentinel, err := p.acquireSentinel(ctx, logf)
	if err != nil {
		logf("[chatgpt] sentinel failed: %v (proceeding without)", err)
		// Non-fatal: try the request anyway; some sessions may not require it.
	}
	model := p.model
	if opts.Model != "" {
		model = opts.Model
	}
	tsl, _ := rand.Int(rand.Reader, big.NewInt(481))
	reqBody := conversationRequest{
		Action: "next",
		Messages: []message{
			{
				ID:     newUUID(),
				Author: author{Role: "user"},
				Content: content{
					ContentType: "text",
					Parts:       []string{query},
				},
				CreateTime: float64(time.Now().Unix()),
				Metadata: messageMetadata{
					SerializationMetadata: serializationMetadata{
						CustomSymbolOffsets: []interface{}{},
					},
				},
			},
		},
		ParentMessageID:            newUUID(),
		Model:                      model,
		TimezoneOffsetMin:          -480,
		Timezone:                   "America/Los_Angeles",
		HistoryAndTrainingDisabled: opts.Temporary,
		ConversationMode:           conversationMode{Kind: "primary_assistant"},
		EnableMessageFollowups:     true,
		SystemHints:                []string{},
		// Don't send supported_encodings/supports_buffering — v1 delta encoding
		// uses a completely different response format we don't parse yet.
		ClientContextualInfo: clientContextualInfo{
			IsDarkMode:      false,
			TimeSinceLoaded: int(tsl.Int64()) + 20,
			PageHeight:      578,
			PageWidth:       1850,
			PixelRatio:      1,
			ScreenHeight:    1080,
			ScreenWidth:     1920,
		},
		ParagenCotSummaryDisplayOverride: "allow",
	}
	if opts.ConversationID != "" {
		reqBody.ConversationID = opts.ConversationID
		reqBody.ParentMessageID = opts.ParentMessageID
	}
	// Apply thinking effort if set.
	if p.thinkingEffort != "" {
		reqBody.ThinkingEffort = p.thinkingEffort
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}
	url := p.baseURL + conversationPath
	logf("[chatgpt] POST %s (model=%s)", url, model)
	logf("[chatgpt] request body: %s", string(payload))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("OAI-Device-Id", p.deviceID)
	req.Header.Set("OAI-Language", "en-US")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")

	// Attach sentinel headers if we obtained them.
	if sentinel != nil {
		req.Header.Set("Openai-Sentinel-Chat-Requirements-Token", sentinel.ChatToken)
		if sentinel.ProofToken != "" {
			req.Header.Set("Openai-Sentinel-Proof-Token", sentinel.ProofToken)
		}
	}

	p.setCookies(req)

	client := httpclient.New(p.timeout)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return p.readStream(resp.Body, opts)
}

func (p *Provider) getAccessToken(ctx context.Context, logf func(string, ...any)) (string, error) {
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		logf("[chatgpt] using cached access token")
		return p.accessToken, nil
	}

	// Try multiple hosts.
	hosts := []string{p.baseURL, "https://chatgpt.com", "https://chat.openai.com"}
	var lastErr error

	for _, host := range hosts {
		url := host + sessionPath
		logf("[chatgpt] refreshing token from %s", url)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("User-Agent", p.userAgent)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		p.setCookies(req)

		client := httpclient.New(p.timeout)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("session endpoint returned %d: %s", resp.StatusCode, string(body))
			continue
		}

		// Check for rotated session token.
		for _, c := range resp.Cookies() {
			if c.Name == cookieSessionToken && c.Value != "" {
				p.sessionToken = c.Value
			}
		}

		var session sessionResponse
		if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("decoding session: %w", err)
			continue
		}
		resp.Body.Close()

		if session.AccessToken == "" {
			lastErr = fmt.Errorf("empty access token — session may be expired")
			continue
		}

		p.accessToken = session.AccessToken
		p.tokenExpiry = time.Now().Add(55 * time.Minute)
		logf("[chatgpt] access token obtained")
		return p.accessToken, nil
	}

	return "", fmt.Errorf("all auth attempts failed: %w", lastErr)
}

func (p *Provider) setCookies(req *http.Request) {
	if p.sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: cookieSessionToken, Value: p.sessionToken})
	}
	if p.cfClearance != "" {
		req.AddCookie(&http.Cookie{Name: cookieCfClearance, Value: p.cfClearance})
	}
	if p.puid != "" {
		req.AddCookie(&http.Cookie{Name: cookiePUID, Value: p.puid})
	}
}

func (p *Provider) readStream(r io.Reader, opts provider.AskOptions) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var fullText string
	var lastConversationID string
	var lastMessageID string

	for scanner.Scan() {
		line := scanner.Text()

		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "[chatgpt-stream] line: %s\n", line)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if opts.OnDone != nil {
				opts.OnDone()
			}
			break
		}

		var frame conversationResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			continue
		}

		if frame.Message == nil || frame.Message.Author.Role != "assistant" {
			continue
		}

		if frame.ConversationID != "" {
			lastConversationID = frame.ConversationID
		}
		if frame.Message.ID != "" {
			lastMessageID = frame.Message.ID
		}

		if len(frame.Message.Content.Parts) > 0 {
			current := frame.Message.Content.Parts[len(frame.Message.Content.Parts)-1]
			if len(current) > len(fullText) {
				delta := current[len(fullText):]
				fullText = current
				if opts.OnText != nil {
					opts.OnText(delta)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stream: %w", err)
	}

	if opts.OnConversation != nil && (lastConversationID != "" || lastMessageID != "") {
		opts.OnConversation(lastConversationID, lastMessageID, "")
	}

	return nil
}

func newUUID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// --- List conversations ---

const conversationsPath = "/backend-api/conversations"

type conversationsResponse struct {
	Items  []conversationItem `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// flexTime handles both Unix epoch (float64) and ISO 8601 string timestamps.
type flexTime struct {
	Time  time.Time
	Valid bool
}

func (ft *flexTime) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), "\"")
	if raw == "" || raw == "null" {
		return nil
	}
	// Try Unix epoch (float64).
	var f float64
	if err := json.Unmarshal([]byte(raw), &f); err == nil && f > 0 {
		ft.Time = time.Unix(int64(f), 0)
		ft.Valid = true
		return nil
	}
	// Try ISO 8601.
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		ft.Time = t
		ft.Valid = true
		return nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		ft.Time = t
		ft.Valid = true
		return nil
	}
	return nil // ignore unparseable timestamps
}

type conversationItem struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	CreateTime flexTime `json:"create_time"`
	UpdateTime flexTime `json:"update_time"`
}

// ListConversations fetches recent conversations from the ChatGPT web API.
func (p *Provider) ListConversations(ctx context.Context, opts provider.ListOptions) ([]provider.Conversation, error) {
	if p.sessionToken == "" {
		return nil, fmt.Errorf("no session cookie — log in to chatgpt.com in your browser")
	}

	logf := opts.LogFunc
	if logf == nil {
		logf = func(string, ...any) {}
	}

	token, err := p.getAccessToken(ctx, logf)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	u := fmt.Sprintf("%s%s?offset=0&limit=%d&order=updated", p.baseURL, conversationsPath, limit)
	logf("[chatgpt] GET %s", u)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "application/json")
	p.setCookies(req)

	client := httpclient.New(p.timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data conversationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	result := make([]provider.Conversation, 0, len(data.Items))
	for _, item := range data.Items {
		c := provider.Conversation{
			ID:    item.ID,
			Title: item.Title,
		}
		if item.CreateTime.Valid {
			c.CreatedAt = item.CreateTime.Time
		}
		if item.UpdateTime.Valid {
			c.UpdatedAt = item.UpdateTime.Time
		}
		result = append(result, c)
	}

	logf("[chatgpt] fetched %d conversations (total %d)", len(result), data.Total)
	return result, nil
}

// --- Model catalog ---

// ListModels returns the available ChatGPT models.
func (p *Provider) ListModels() provider.ProviderModels {
	return provider.ProviderModels{
		Provider: "chatgpt",
		Models: []provider.ModelInfo{
			{ID: "auto", Name: "Auto", Description: "Automatic model selection", Default: false, Tags: []string{"auto"}},
			{ID: "gpt-5.2", Name: "GPT-5.2", Description: "Latest flagship model", Default: false, Tags: []string{"flagship"}},
			{ID: "gpt-5.2-instant", Name: "GPT-5.2 Instant", Description: "Fast, no thinking", Default: false, Tags: []string{"fast"}},
			{ID: "gpt-5.2-thinking", Name: "GPT-5.2 Thinking", Description: "With reasoning/thinking enabled", Default: true, Tags: []string{"reasoning"}},
			{ID: "gpt-5-nano", Name: "GPT-5 Nano", Description: "Lightweight model", Default: false, Tags: []string{"fast", "lightweight"}},
			{ID: "o3-mini", Name: "o3-mini", Description: "Reasoning model (standard)", Default: false, Tags: []string{"reasoning"}},
			{ID: "o3-mini-high", Name: "o3-mini-high", Description: "Reasoning model (high effort)", Default: false, Tags: []string{"reasoning"}},
		},
		Modes: []provider.ModeInfo{
			{ID: "none", Name: "None", Description: "No thinking", Default: false},
			{ID: "low", Name: "Low", Description: "Light reasoning effort", Default: false},
			{ID: "medium", Name: "Medium", Description: "Standard reasoning effort", Default: false},
			{ID: "high", Name: "High", Description: "Extended reasoning effort", Default: false},
			{ID: "xhigh", Name: "Extra High", Description: "Heavy reasoning effort", Default: true},
		},
	}
}

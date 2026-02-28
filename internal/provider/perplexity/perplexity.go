// Package perplexity implements the Perplexity AI provider.
package perplexity

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kyupark/ask/internal/httpclient"
	"github.com/kyupark/ask/internal/provider"
	"github.com/kyupark/ask/internal/sse"
)

const (
	defaultBaseURL   = "https://www.perplexity.ai"
	askEndpoint      = "/rest/sse/perplexity_ask"
	threadPath       = "/rest/thread"
	deleteThreadPath = "/rest/thread/delete_thread_by_entry_uuid"

	cookieCfClearance  = "cf_clearance"
	cookieSessionToken = "__Secure-next-auth.session-token"
	domainSuffix       = "perplexity.ai"
)

// askRequest is the POST body for the ask endpoint.
type askRequest struct {
	QueryStr string    `json:"query_str"`
	Params   askParams `json:"params"`
}

type askParams struct {
	Attachments         []string `json:"attachments"`
	FrontendContextUUID string   `json:"frontend_context_uuid"`
	FrontendUUID        string   `json:"frontend_uuid"`
	IsIncognito         bool     `json:"is_incognito"`
	Language            string   `json:"language"`
	Mode                string   `json:"mode"`
	ModelPreference     string   `json:"model_preference,omitempty"`
	Source              string   `json:"source"`
	Sources             []string `json:"sources"`
	SearchFocus         string   `json:"search_focus"`
	Version             string   `json:"version"`
}

// askResponse is a single SSE event from the ask endpoint.
type askResponse struct {
	Blocks []block `json:"blocks"`
	Status string  `json:"status"`
}

type block struct {
	MarkdownBlock  *markdownBlock  `json:"markdown_block,omitempty"`
	WebResultBlock *webResultBlock `json:"web_result_block,omitempty"`
}

type markdownBlock struct {
	Chunks []string `json:"chunks"`
}

type webResultBlock struct {
	WebResults []webResult `json:"web_results"`
}

type webResult struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Provider implements the Perplexity AI backend.
type Provider struct {
	baseURL       string
	userAgent     string
	timeout       time.Duration
	cfClearance   string
	sessionCookie string
	modeOverride  string
	focusOverride string
}

// New creates a Perplexity provider with the given settings.
func New(baseURL, userAgent string, timeout time.Duration) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Provider{
		baseURL:   baseURL,
		userAgent: userAgent,
		timeout:   timeout,
	}
}

func (p *Provider) Name() string { return "perplexity" }

func (p *Provider) CookieSpecs() []provider.CookieSpec {
	return []provider.CookieSpec{
		{Domain: domainSuffix, Names: []string{cookieCfClearance, cookieSessionToken}},
	}
}

func (p *Provider) SetCookies(cookies map[string]string) {
	if v := cookies[cookieCfClearance]; v != "" {
		p.cfClearance = v
	}
	if v := cookies[cookieSessionToken]; v != "" {
		p.sessionCookie = v
	}
}

// SetMode overrides the default mode (auto, pro, reasoning, deep research).
func (p *Provider) SetMode(mode string) { p.modeOverride = mode }

// SetSearchFocus overrides the default search focus (internet, scholar, social, edgar, writing).
func (p *Provider) SetSearchFocus(focus string) { p.focusOverride = focus }

func (p *Provider) Ask(ctx context.Context, query string, opts provider.AskOptions) error {
	if p.sessionCookie == "" {
		return fmt.Errorf("no session cookie — log in to perplexity.ai in your browser")
	}

	logf := opts.LogFunc
	if logf == nil {
		logf = func(string, ...any) {}
	}

	reqBody := askRequest{
		QueryStr: query,
		Params: askParams{
			Attachments:         []string{},
			FrontendContextUUID: generateUUID(),
			FrontendUUID:        generateUUID(),
			IsIncognito:         opts.Temporary,
			Language:            "en-US",
			Mode:                "reasoning",
			Source:              "default",
			Sources:             []string{"web"},
			SearchFocus:         "internet",
			Version:             "2.18",
		},
	}
	if opts.ConversationID != "" {
		reqBody.Params.FrontendContextUUID = opts.ConversationID
	}
	if opts.Model != "" {
		reqBody.Params.ModelPreference = opts.Model
	}
	if p.modeOverride != "" {
		reqBody.Params.Mode = p.modeOverride
	}
	if p.focusOverride != "" {
		reqBody.Params.SearchFocus = p.focusOverride
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}

	url := p.baseURL + askEndpoint
	logf("[perplexity] POST %s", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", p.baseURL)
	req.Header.Set("Referer", p.baseURL+"/")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", p.userAgent)

	if p.cfClearance != "" {
		req.AddCookie(&http.Cookie{Name: cookieCfClearance, Value: p.cfClearance})
	}
	req.AddCookie(&http.Cookie{Name: cookieSessionToken, Value: p.sessionCookie})

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

	// Track total text length for delta — the API sends cumulative
	// chunks where each event repeats prior text.
	var totalPrinted int

	err = sse.Read(resp.Body, func(event sse.Event) error {
		var r askResponse
		if err := json.Unmarshal([]byte(event.Data), &r); err != nil {
			if opts.OnError != nil {
				opts.OnError(fmt.Errorf("parsing event: %w", err))
			}
			return nil // non-fatal
		}

		for _, b := range r.Blocks {
			if b.MarkdownBlock != nil && opts.OnText != nil {
				var full string
				for _, chunk := range b.MarkdownBlock.Chunks {
					full += chunk
				}
				if len(full) > totalPrinted {
					opts.OnText(full[totalPrinted:])
					totalPrinted = len(full)
				}
			}
			if b.WebResultBlock != nil && opts.OnSource != nil {
				for _, src := range b.WebResultBlock.WebResults {
					opts.OnSource(src.Name, src.URL)
				}
			}
		}

		if r.Status == "COMPLETED" {
			if opts.OnDone != nil {
				opts.OnDone()
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	if opts.OnConversation != nil {
		opts.OnConversation(reqBody.Params.FrontendContextUUID, "", "")
	}
	return nil
}

func generateUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// --- List conversations ---

const listThreadsPath = "/rest/thread/list_ask_threads"

type listThreadsRequest struct {
	Limit      int    `json:"limit"`
	Ascending  bool   `json:"ascending"`
	Offset     int    `json:"offset"`
	SearchTerm string `json:"search_term"`
}

type threadItem struct {
	ContextUUID         string `json:"context_uuid"`
	FrontendContextUUID string `json:"frontend_context_uuid"`
	Title               string `json:"title"`
	LastQueryDatetime   string `json:"last_query_datetime"`
	Slug                string `json:"slug"`
	ReadWriteToken      string `json:"read_write_token"`
}

type threadDetails struct {
	Entries []threadEntry `json:"entries"`
}

type threadEntry struct {
	BackendUUID    string `json:"backend_uuid"`
	ReadWriteToken string `json:"read_write_token"`
}

// ListConversations fetches recent threads from the Perplexity web API.
func (p *Provider) ListConversations(ctx context.Context, opts provider.ListOptions) ([]provider.Conversation, error) {
	if p.sessionCookie == "" {
		return nil, fmt.Errorf("no session cookie — log in to perplexity.ai in your browser")
	}

	logf := opts.LogFunc
	if logf == nil {
		logf = func(string, ...any) {}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	reqBody := listThreadsRequest{
		Limit:      limit,
		Ascending:  false,
		Offset:     0,
		SearchTerm: "",
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	u := p.baseURL + listThreadsPath + "?version=2.18&source=default"
	logf("[perplexity] POST %s", u)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("X-App-Apiclient", "default")
	req.Header.Set("X-App-Apiversion", "2.18")
	req.Header.Set("Origin", p.baseURL)
	req.Header.Set("Referer", p.baseURL+"/")
	if p.cfClearance != "" {
		req.AddCookie(&http.Cookie{Name: cookieCfClearance, Value: p.cfClearance})
	}
	req.AddCookie(&http.Cookie{Name: cookieSessionToken, Value: p.sessionCookie})

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

	var threads []threadItem
	if err := json.NewDecoder(resp.Body).Decode(&threads); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	result := make([]provider.Conversation, 0, len(threads))
	for _, t := range threads {
		c := provider.Conversation{
			ID:    t.ContextUUID,
			Title: t.Title,
		}
		if t.LastQueryDatetime != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, t.LastQueryDatetime); err == nil {
				c.CreatedAt = parsed
			} else if parsed, err := time.Parse(time.RFC3339, t.LastQueryDatetime); err == nil {
				c.CreatedAt = parsed
			}
		}
		result = append(result, c)
	}

	logf("[perplexity] fetched %d threads", len(result))
	return result, nil
}

func (p *Provider) DeleteConversation(ctx context.Context, conversationID string, opts provider.DeleteOptions) error {
	if p.sessionCookie == "" {
		return fmt.Errorf("no session cookie — log in to perplexity.ai in your browser")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return fmt.Errorf("conversation ID is required")
	}

	logf := opts.LogFunc
	if logf == nil {
		logf = func(string, ...any) {}
	}

	thread, err := p.findThreadByContextID(ctx, conversationID)
	if err != nil {
		return err
	}

	entryUUID := ""
	readWriteToken := strings.TrimSpace(thread.ReadWriteToken)
	if strings.TrimSpace(thread.Slug) != "" {
		details, err := p.fetchThreadDetails(ctx, thread.Slug)
		if err != nil {
			logf("[perplexity] unable to fetch thread details for slug=%s: %v", thread.Slug, err)
		} else {
			for _, entry := range details.Entries {
				if strings.TrimSpace(entryUUID) == "" && strings.TrimSpace(entry.BackendUUID) != "" {
					entryUUID = strings.TrimSpace(entry.BackendUUID)
				}
				if strings.TrimSpace(readWriteToken) == "" && strings.TrimSpace(entry.ReadWriteToken) != "" {
					readWriteToken = strings.TrimSpace(entry.ReadWriteToken)
				}
				if entryUUID != "" && readWriteToken != "" {
					break
				}
			}
		}
	}

	if entryUUID == "" {
		return fmt.Errorf("could not resolve entry UUID for conversation %s", conversationID)
	}
	if readWriteToken == "" {
		return fmt.Errorf("could not resolve read/write token for conversation %s", conversationID)
	}

	deletePayload, err := json.Marshal(map[string]string{
		"entry_uuid":       entryUUID,
		"read_write_token": readWriteToken,
	})
	if err != nil {
		return fmt.Errorf("marshalling delete payload: %w", err)
	}

	u := p.baseURL + deleteThreadPath + "?version=2.18&source=default"
	logf("[perplexity] DELETE %s", u)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, bytes.NewReader(deletePayload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-App-Apiclient", "default")
	req.Header.Set("X-App-Apiversion", "2.18")
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Origin", p.baseURL)
	req.Header.Set("Referer", p.baseURL+"/")
	if p.cfClearance != "" {
		req.AddCookie(&http.Cookie{Name: cookieCfClearance, Value: p.cfClearance})
	}
	req.AddCookie(&http.Cookie{Name: cookieSessionToken, Value: p.sessionCookie})

	client := httpclient.New(p.timeout)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	logf("[perplexity] conversation deleted")
	return nil
}

func (p *Provider) findThreadByContextID(ctx context.Context, contextID string) (*threadItem, error) {
	contextID = strings.TrimSpace(contextID)
	if contextID == "" {
		return nil, fmt.Errorf("conversation ID is required")
	}

	client := httpclient.New(p.timeout)
	for offset := 0; offset < 1000; offset += 50 {
		reqBody := listThreadsRequest{Limit: 50, Ascending: false, Offset: offset, SearchTerm: ""}
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshalling request: %w", err)
		}

		u := p.baseURL + listThreadsPath + "?version=2.18&source=default"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("User-Agent", p.userAgent)
		req.Header.Set("X-App-Apiclient", "default")
		req.Header.Set("X-App-Apiversion", "2.18")
		req.Header.Set("Origin", p.baseURL)
		req.Header.Set("Referer", p.baseURL+"/")
		if p.cfClearance != "" {
			req.AddCookie(&http.Cookie{Name: cookieCfClearance, Value: p.cfClearance})
		}
		req.AddCookie(&http.Cookie{Name: cookieSessionToken, Value: p.sessionCookie})

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		var threads []threadItem
		if err := json.NewDecoder(resp.Body).Decode(&threads); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		for _, t := range threads {
			if strings.TrimSpace(t.ContextUUID) == contextID || strings.TrimSpace(t.FrontendContextUUID) == contextID {
				item := t
				return &item, nil
			}
		}

		if len(threads) < 50 {
			break
		}
	}

	return nil, fmt.Errorf("conversation %s not found", contextID)
}

func (p *Provider) fetchThreadDetails(ctx context.Context, slug string) (*threadDetails, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("thread slug is required")
	}

	params := url.Values{}
	params.Set("with_parent_info", "true")
	params.Set("with_schematized_response", "true")
	params.Set("version", "2.18")
	params.Set("source", "default")
	params.Set("limit", "10")
	params.Set("offset", "0")
	params.Set("from_first", "true")
	u := fmt.Sprintf("%s%s/%s?%s", p.baseURL, threadPath, slug, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-App-Apiclient", "default")
	req.Header.Set("X-App-Apiversion", "2.18")
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Origin", p.baseURL)
	req.Header.Set("Referer", p.baseURL+"/")
	if p.cfClearance != "" {
		req.AddCookie(&http.Cookie{Name: cookieCfClearance, Value: p.cfClearance})
	}
	req.AddCookie(&http.Cookie{Name: cookieSessionToken, Value: p.sessionCookie})

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

	var details threadDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &details, nil
}

// --- Model catalog ---

// ListModels returns the available Perplexity models, modes, and search focuses.
func (p *Provider) ListModels() provider.ProviderModels {
	return provider.ProviderModels{
		Provider: "perplexity",
		Models: []provider.ModelInfo{
			{ID: "turbo", Name: "Auto (Turbo)", Description: "Automatic model selection", Default: false, Tags: []string{"auto"}},
			{ID: "pplx_pro", Name: "Pro", Description: "Default Pro mode model", Default: false, Tags: []string{"pro"}},
			{ID: "pplx_reasoning", Name: "Reasoning", Description: "Default Reasoning mode model", Default: true, Tags: []string{"reasoning"}},
			{ID: "pplx_alpha", Name: "Deep Research", Description: "Deep research agent", Default: false, Tags: []string{"deep-research"}},
			{ID: "pplx_beta", Name: "Labs", Description: "Experimental models", Default: false, Tags: []string{"labs"}},
			{ID: "experimental", Name: "Sonar", Description: "Sonar model", Default: false, Tags: []string{"pro"}},
			{ID: "gpt52", Name: "GPT-5.2", Description: "OpenAI GPT-5.2 via Perplexity", Default: false, Tags: []string{"pro", "external"}},
			{ID: "claude45sonnet", Name: "Claude 4.5 Sonnet", Description: "Anthropic Claude via Perplexity", Default: false, Tags: []string{"pro", "external"}},
			{ID: "grok41nonreasoning", Name: "Grok 4.1", Description: "xAI Grok via Perplexity", Default: false, Tags: []string{"pro", "external"}},
			{ID: "gpt52_thinking", Name: "GPT-5.2 Thinking", Description: "OpenAI GPT-5.2 with thinking", Default: false, Tags: []string{"reasoning", "external"}},
			{ID: "claude45sonnetthinking", Name: "Claude 4.5 Sonnet Thinking", Description: "Claude with thinking", Default: false, Tags: []string{"reasoning", "external"}},
			{ID: "gemini30pro", Name: "Gemini 3.0 Pro", Description: "Google Gemini via Perplexity", Default: false, Tags: []string{"reasoning", "external"}},
			{ID: "kimik2thinking", Name: "Kimi K2 Thinking", Description: "Kimi K2 with thinking", Default: false, Tags: []string{"reasoning", "external"}},
			{ID: "grok41reasoning", Name: "Grok 4.1 Reasoning", Description: "Grok with reasoning", Default: false, Tags: []string{"reasoning", "external"}},
		},
		Modes: []provider.ModeInfo{
			{ID: "auto", Name: "Auto", Description: "Automatic mode selection", Default: false},
			{ID: "pro", Name: "Pro", Description: "Professional search with citations", Default: false},
			{ID: "reasoning", Name: "Reasoning", Description: "Thinking/reasoning mode", Default: true},
			{ID: "deep research", Name: "Deep Research", Description: "Multi-step deep research agent", Default: false},
		},
		SearchFocus: []provider.ModeInfo{
			{ID: "internet", Name: "Internet", Description: "General web search", Default: true},
			{ID: "scholar", Name: "Scholar", Description: "Academic papers and research", Default: false},
			{ID: "social", Name: "Social", Description: "Social media and forums", Default: false},
			{ID: "edgar", Name: "EDGAR", Description: "SEC EDGAR financial filings", Default: false},
			{ID: "writing", Name: "Writing", Description: "Writing assistant (no search)", Default: false},
		},
	}
}

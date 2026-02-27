package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName    = "ask"
	configFile = "config.json"

	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	DefaultTimeout   = 180
)

// Config is the top-level configuration.
type Config struct {
	UserAgent string `json:"user_agent,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`
	Verbose   bool   `json:"verbose,omitempty"`

	Perplexity PerplexityConfig `json:"perplexity,omitempty"`
	ChatGPT    ChatGPTConfig    `json:"chatgpt,omitempty"`
	Gemini     GeminiConfig     `json:"gemini,omitempty"`
	Grok       GrokConfig       `json:"grok,omitempty"`
	Claude     ClaudeConfig     `json:"claude,omitempty"`
}

// PerplexityConfig holds Perplexity-specific settings.
type PerplexityConfig struct {
	CfClearance   string `json:"cf_clearance,omitempty"`
	SessionCookie string `json:"session_cookie,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	Model         string `json:"model,omitempty"`
	Mode          string `json:"mode,omitempty"`
	SearchFocus   string `json:"search_focus,omitempty"`
}

// ChatGPTConfig holds ChatGPT-specific settings.
type ChatGPTConfig struct {
	SessionToken string `json:"session_token,omitempty"`
	CfClearance  string `json:"cf_clearance,omitempty"`
	PUID         string `json:"puid,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	Model        string `json:"model,omitempty"`
	Effort       string `json:"effort,omitempty"`
}

// GeminiConfig holds Gemini-specific settings.
type GeminiConfig struct {
	PSID   string `json:"psid,omitempty"`
	PSIDTS string `json:"psidts,omitempty"`
	PSIDCC string `json:"psidcc,omitempty"`
	Model  string `json:"model,omitempty"`
}

// GrokConfig holds Grok (X.com) specific settings.
type GrokConfig struct {
	AuthToken  string `json:"auth_token,omitempty"`
	CT0        string `json:"ct0,omitempty"`
	Model      string `json:"model,omitempty"`
	DeepSearch bool   `json:"deepsearch,omitempty"`
	Reasoning  bool   `json:"reasoning,omitempty"`
}

// ClaudeConfig holds Claude.ai specific settings.
type ClaudeConfig struct {
	SessionKey string `json:"session_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"`
}

// Load reads config from the XDG config file, applying defaults.
func Load() *Config {
	cfg := &Config{
		UserAgent: DefaultUserAgent,
		Timeout:   DefaultTimeout,
	}

	path := FilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	_ = json.Unmarshal(data, cfg)

	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	return cfg
}

// Save writes the config to the XDG config file.
func Save(cfg *Config) error {
	path := FilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

// FilePath returns the path to the config file.
func FilePath() string {
	return filePathForApp(appName, configFile)
}

func filePathForApp(app, file string) string {
	return filepath.Join(configBaseDir(), app, file)
}

func configBaseDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "."
		}
		dir = filepath.Join(home, ".config")
	}
	return dir
}

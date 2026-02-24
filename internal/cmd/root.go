package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/qm4/webai-cli/internal/config"
	"github.com/qm4/webai-cli/internal/cookies"
	"github.com/qm4/webai-cli/internal/provider"
)

var (
	globalCfg   *config.Config
	flagVerbose bool
)

var rootCmd = &cobra.Command{
	Use:   "chatmux",
	Short: "Unified CLI for AI chatbots (Perplexity, ChatGPT, Gemini, Grok, Claude)",
	Long: `chatmux provides a single interface to multiple AI chatbots using
browser cookie authentication. No API keys required.

Supported providers:
  perplexity  — Perplexity AI (SSE streaming)
  chatgpt     — ChatGPT / OpenAI (SSE streaming)
  gemini      — Google Gemini (batch RPC)
  grok        — Grok / X.com (NDJSON streaming)
  claude      — Claude.ai / Anthropic (SSE streaming)
Usage:
  chatmux perplexity ask "your question"
  chatmux chatgpt ask-incognito "your question"
  chatmux gemini list
  chatmux install-openclaw-skill
Cookies are auto-extracted from Safari (preferred) or Chrome.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		globalCfg = config.Load()
		if flagVerbose {
			globalCfg.Verbose = true
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// autoLoadCookies extracts cookies for the provider from browsers if needed.
func autoLoadCookies(ctx context.Context, p provider.Provider) {
	specs := p.CookieSpecs()
	if len(specs) == 0 {
		return
	}

	logf := func(string, ...any) {}
	if globalCfg.Verbose {
		logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	// Convert provider.CookieSpec to cookies.Spec.
	var cookieSpecs []cookies.Spec
	for _, s := range specs {
		cookieSpecs = append(cookieSpecs, cookies.Spec{
			Domain: s.Domain,
			Names:  s.Names,
		})
	}

	result, err := cookies.ExtractMulti(ctx, cookieSpecs, logf)
	if err != nil {
		if globalCfg.Verbose {
			fmt.Fprintf(os.Stderr, "[autoload] cookie extraction error: %v\n", err)
		}
		return
	}

	if len(result.Cookies) > 0 {
		p.SetCookies(result.Cookies)
		if globalCfg.Verbose {
			fmt.Fprintf(os.Stderr, "[autoload] loaded %d cookies from %s\n", len(result.Cookies), result.Browser)
		}
	}
}

// providerTimeout returns the configured timeout as time.Duration.
func providerTimeout() time.Duration {
	return time.Duration(globalCfg.Timeout) * time.Second
}

package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	cfgpkg "github.com/qm4/webai-cli/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage default config",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print current config as JSON",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		masked := *globalCfg
		masked.Perplexity.CfClearance = maskSecret(masked.Perplexity.CfClearance)
		masked.Perplexity.SessionCookie = maskSecret(masked.Perplexity.SessionCookie)
		masked.ChatGPT.SessionToken = maskSecret(masked.ChatGPT.SessionToken)
		masked.ChatGPT.CfClearance = maskSecret(masked.ChatGPT.CfClearance)
		masked.ChatGPT.PUID = maskSecret(masked.ChatGPT.PUID)
		masked.Gemini.PSID = maskSecret(masked.Gemini.PSID)
		masked.Gemini.PSIDTS = maskSecret(masked.Gemini.PSIDTS)
		masked.Gemini.PSIDCC = maskSecret(masked.Gemini.PSIDCC)
		masked.Grok.AuthToken = maskSecret(masked.Grok.AuthToken)
		masked.Grok.CT0 = maskSecret(masked.Grok.CT0)
		masked.Claude.SessionKey = maskSecret(masked.Claude.SessionKey)

		out, err := json.MarshalIndent(masked, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), string(out))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := strings.ToLower(args[0])
		value := args[1]

		switch key {
		case "chatgpt.model":
			globalCfg.ChatGPT.Model = value
		case "chatgpt.effort":
			globalCfg.ChatGPT.Effort = value
		case "claude.model":
			globalCfg.Claude.Model = value
		case "claude.effort":
			globalCfg.Claude.Effort = value
		case "perplexity.model":
			globalCfg.Perplexity.Model = value
		case "perplexity.mode":
			globalCfg.Perplexity.Mode = value
		case "perplexity.focus":
			globalCfg.Perplexity.SearchFocus = value
		case "gemini.model":
			globalCfg.Gemini.Model = value
		case "grok.model":
			globalCfg.Grok.Model = value
		case "grok.deepsearch":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool for %s: %q", key, value)
			}
			globalCfg.Grok.DeepSearch = parsed
		case "grok.reasoning":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool for %s: %q", key, value)
			}
			globalCfg.Grok.Reasoning = parsed
		case "timeout":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid int for %s: %q", key, value)
			}
			globalCfg.Timeout = parsed
		case "verbose":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid bool for %s: %q", key, value)
			}
			globalCfg.Verbose = parsed
		default:
			return fmt.Errorf("unsupported config key: %s", key)
		}

		if err := cfgpkg.Save(globalCfg); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "set %s=%s\n", key, value)
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print config file path",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), cfgpkg.FilePath())
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}

func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	return "***"
}

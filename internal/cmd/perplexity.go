package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyupark/ask/internal/config"
	"github.com/kyupark/ask/internal/provider"
	"github.com/kyupark/ask/internal/provider/perplexity"
)

var (
	perplexityModel        string
	perplexityMode         string
	perplexityFocus        string
	perplexityResume       bool
	perplexityConversation string
)

var perplexityCmd = &cobra.Command{
	Use:   "perplexity [question]",
	Short: "Perplexity AI commands",
	Long: `Interact with Perplexity AI using browser cookies.

Subcommands:
  <question>      Ask a question (saves to history)
  ask-incognito  Ask a question (no history)
  list           List recent threads
	delete         Delete a thread by ID
	models         Show available models, modes, and search focuses`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runPerplexityAsk(cmd, args, false)
	},
}

var perplexityAskIncognitoCmd = &cobra.Command{
	Use:   "ask-incognito [question]",
	Short: "Ask Perplexity (no history)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runPerplexityAsk(cmd, args, true) },
}

var perplexityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent Perplexity threads",
	Args:  cobra.NoArgs,
	RunE:  runPerplexityList,
}

var perplexityDeleteCmd = &cobra.Command{
	Use:   "delete <conversation-id>",
	Short: "Delete a Perplexity thread",
	Args:  cobra.ExactArgs(1),
	RunE:  runPerplexityDelete,
}

var perplexityModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Show available Perplexity models and modes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := perplexity.New("", "", providerTimeout())
		return runModels(p)
	},
}

func init() {
	perplexityCmd.Flags().StringVarP(&perplexityModel, "model", "m", "", "Model preference (e.g. 'pplx_reasoning', 'gpt52')")
	perplexityCmd.Flags().StringVar(&perplexityMode, "mode", "", "Mode (auto, pro, reasoning, deep research)")
	perplexityCmd.Flags().StringVar(&perplexityFocus, "focus", "", "Search focus (internet, scholar, social, edgar, writing)")
	perplexityCmd.Flags().BoolVarP(&perplexityResume, "resume", "r", false, "Resume last conversation")
	perplexityCmd.Flags().StringVar(&perplexityConversation, "conversation", "", "Continue a specific conversation by ID")
	perplexityAskIncognitoCmd.Flags().StringVarP(&perplexityModel, "model", "m", "", "Model preference (e.g. 'pplx_reasoning', 'gpt52')")
	perplexityAskIncognitoCmd.Flags().StringVar(&perplexityMode, "mode", "", "Mode (auto, pro, reasoning, deep research)")
	perplexityAskIncognitoCmd.Flags().StringVar(&perplexityFocus, "focus", "", "Search focus (internet, scholar, social, edgar, writing)")
	perplexityCmd.AddCommand(perplexityAskIncognitoCmd)
	perplexityCmd.AddCommand(perplexityListCmd)
	perplexityCmd.AddCommand(perplexityDeleteCmd)
	perplexityCmd.AddCommand(perplexityModelsCmd)
	rootCmd.AddCommand(perplexityCmd)
}

func runPerplexityAsk(cmd *cobra.Command, args []string, temporary bool) error {
	query := strings.Join(args, " ")

	model := globalCfg.Perplexity.Model
	if perplexityModel != "" {
		model = perplexityModel
	}

	mode := perplexityMode
	if mode == "" {
		mode = globalCfg.Perplexity.Mode
	}

	focus := perplexityFocus
	if focus == "" {
		focus = globalCfg.Perplexity.SearchFocus
	}

	p := perplexity.New(
		globalCfg.Perplexity.BaseURL,
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"cf_clearance":                     globalCfg.Perplexity.CfClearance,
		"__Secure-next-auth.session-token": globalCfg.Perplexity.SessionCookie,
	})

	autoLoadCookies(cmd.Context(), p)

	// Apply mode/focus overrides if set.
	if mode != "" {
		p.SetMode(mode)
	}
	if focus != "" {
		p.SetSearchFocus(focus)
	}

	var sources []struct{ name, url string }

	opts := provider.AskOptions{
		Model:     model,
		Verbose:   globalCfg.Verbose,
		Temporary: temporary,
		OnText: func(text string) {
			fmt.Print(text)
		},
		OnSource: func(name, url string) {
			sources = append(sources, struct{ name, url string }{name, url})
		},
		OnError: func(err error) {
			if globalCfg.Verbose {
				fmt.Fprintf(os.Stderr, "[perplexity] parse error: %v\n", err)
			}
		},
	}

	if !temporary {
		if perplexityConversation != "" {
			opts.ConversationID = perplexityConversation
		} else if perplexityResume {
			state := config.LoadState()
			if conv := state.GetConversation("perplexity"); conv != nil {
				opts.ConversationID = conv.ConversationID
			} else {
				fmt.Fprintln(os.Stderr, "No previous conversation found for perplexity — starting new")
			}
		}
	}

	// Save conversation state and capture ID for hint.
	var lastConvID string
	if !temporary {
		opts.OnConversation = func(convID, parentMsgID, respID string) {
			lastConvID = convID
			state := config.LoadState()
			state.SetConversation("perplexity", &config.ConversationState{
				ConversationID: convID,
			})
			_ = config.SaveState(state)
		}
	}
	if globalCfg.Verbose {
		opts.LogFunc = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	if err := p.Ask(cmd.Context(), query, opts); err != nil {
		return err
	}

	fmt.Println()

	if len(sources) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Sources:")
		for i, src := range sources {
			fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, src.name)
			fmt.Fprintf(os.Stderr, "      %s\n", src.url)
		}
	}

	if lastConvID != "" && !temporary {
		fmt.Fprintf(os.Stderr, "\nConversation: %s\n", lastConvID)
		fmt.Fprintf(os.Stderr, "  ask perplexity -c %s \"follow up\"\n", lastConvID)
	}

	return nil
}

func runPerplexityList(cmd *cobra.Command, args []string) error {
	p := perplexity.New(
		globalCfg.Perplexity.BaseURL,
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"cf_clearance":                     globalCfg.Perplexity.CfClearance,
		"__Secure-next-auth.session-token": globalCfg.Perplexity.SessionCookie,
	})

	return runList(cmd.Context(), p, 20)
}

func runPerplexityDelete(cmd *cobra.Command, args []string) error {
	p := perplexity.New(
		globalCfg.Perplexity.BaseURL,
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"cf_clearance":                     globalCfg.Perplexity.CfClearance,
		"__Secure-next-auth.session-token": globalCfg.Perplexity.SessionCookie,
	})

	return runDelete(cmd.Context(), p, args[0])
}

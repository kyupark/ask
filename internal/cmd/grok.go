package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyupark/ask/internal/config"
	"github.com/kyupark/ask/internal/provider"
	grokpkg "github.com/kyupark/ask/internal/provider/grok"
)

var (
	grokModel        string
	grokDeepsearch   bool
	grokReasoning    bool
	grokResume       bool
	grokConversation string
)

var grokCmd = &cobra.Command{
	Use:   "grok [question]",
	Short: "Grok (X.com) commands",
	Long: `Interact with Grok on X.com using browser cookies.
  <question>      Ask a question (saves to history)
  ask-incognito  Ask a question (no local resume state)
  list           List recent conversations
  models         Show available models
Model aliases: auto, fast, expert, thinking, 4.20, 4, 3, 2, mini`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runGrokAsk(cmd, args, false)
	},
}

var grokAskIncognitoCmd = &cobra.Command{
	Use:   "ask-incognito [question]",
	Short: "Ask Grok (no local resume state)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runGrokAsk(cmd, args, true) },
}

var grokListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent Grok conversations",
	Args:  cobra.NoArgs,
	RunE:  runGrokList,
}

var grokModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Show available Grok models and modes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := grokpkg.New("", providerTimeout())
		return runModels(p)
	},
}

func init() {
	grokCmd.Flags().StringVarP(&grokModel, "model", "m", "", "Model override (e.g. 'auto', '4.20', 'fast', 'expert', 'thinking')")
	grokCmd.Flags().BoolVar(&grokDeepsearch, "deepsearch", false, "Enable DeepSearch mode")
	grokCmd.Flags().BoolVar(&grokReasoning, "reasoning", false, "Enable Reasoning mode")
	grokCmd.Flags().BoolVarP(&grokResume, "resume", "r", false, "Resume last conversation")
	grokCmd.Flags().StringVar(&grokConversation, "conversation", "", "Continue a specific conversation by ID")
	grokAskIncognitoCmd.Flags().StringVarP(&grokModel, "model", "m", "", "Model override (e.g. 'auto', '4.20', 'fast', 'expert', 'thinking')")
	grokAskIncognitoCmd.Flags().BoolVar(&grokDeepsearch, "deepsearch", false, "Enable DeepSearch mode")
	grokAskIncognitoCmd.Flags().BoolVar(&grokReasoning, "reasoning", false, "Enable Reasoning mode")
	grokCmd.AddCommand(grokAskIncognitoCmd)
	grokCmd.AddCommand(grokListCmd)
	grokCmd.AddCommand(grokModelsCmd)
	rootCmd.AddCommand(grokCmd)
}

func runGrokAsk(cmd *cobra.Command, args []string, temporary bool) error {
	query := strings.Join(args, " ")

	p := grokpkg.New(
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"auth_token": globalCfg.Grok.AuthToken,
		"ct0":        globalCfg.Grok.CT0,
	})

	autoLoadCookies(cmd.Context(), p)

	// Apply mode overrides.
	if cmd.Flags().Changed("deepsearch") {
		if grokDeepsearch {
			p.SetDeepSearch(true)
		}
	} else if globalCfg.Grok.DeepSearch {
		p.SetDeepSearch(true)
	}

	if cmd.Flags().Changed("reasoning") {
		if grokReasoning {
			p.SetReasoning(true)
		}
	} else if globalCfg.Grok.Reasoning {
		p.SetReasoning(true)
	}
	model := globalCfg.Grok.Model
	if grokModel != "" {
		model = grokModel
	}

	opts := provider.AskOptions{
		Model:     model,
		Verbose:   globalCfg.Verbose,
		Temporary: temporary,
		OnText: func(text string) {
			fmt.Print(text)
		},
		OnError: func(err error) {
			if globalCfg.Verbose {
				fmt.Fprintf(os.Stderr, "[grok] error: %v\n", err)
			}
		},
	}

	if temporary {
		fmt.Fprintln(os.Stderr, "Note: Grok incognito disables local resume state only; X may still keep server-side conversation history.")
	}

	if !temporary {
		if grokConversation != "" {
			opts.ConversationID = grokConversation
		} else if grokResume {
			state := config.LoadState()
			if conv := state.GetConversation("grok"); conv != nil {
				opts.ConversationID = conv.ConversationID
			} else {
				fmt.Fprintln(os.Stderr, "No previous conversation found for grok — starting new")
			}
		}
	}

	// Save conversation state and capture ID for hint.
	var lastConvID string
	if !temporary {
		opts.OnConversation = func(convID, parentMsgID, respID string) {
			lastConvID = convID
			state := config.LoadState()
			state.SetConversation("grok", &config.ConversationState{
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

	if lastConvID != "" && !temporary {
		fmt.Fprintf(os.Stderr, "\nConversation: %s\n", lastConvID)
		fmt.Fprintf(os.Stderr, "  ask grok -c %s \"follow up\"\n", lastConvID)
	}

	return nil
}

func runGrokList(cmd *cobra.Command, args []string) error {
	p := grokpkg.New(
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"auth_token": globalCfg.Grok.AuthToken,
		"ct0":        globalCfg.Grok.CT0,
	})

	return runList(cmd.Context(), p, 20)
}

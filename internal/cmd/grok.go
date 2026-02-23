package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qm4/webai-cli/internal/config"
	"github.com/qm4/webai-cli/internal/provider"
	grokpkg "github.com/qm4/webai-cli/internal/provider/grok"
)

var (
	grokModel        string
	grokDeepsearch   bool
	grokReasoning    bool
	grokResume       bool
	grokConversation string
)

var grokCmd = &cobra.Command{
	Use:   "grok",
	Short: "Grok (X.com) commands",
	Long: `Interact with Grok on X.com using browser cookies.
  ask            Ask a question (saves to history)
  ask-incognito  Ask a question (no local resume state)
  list           List recent conversations
  models         Show available models
Model aliases: auto, fast, expert, thinking, 4, 3, 2, mini`,
}

var grokAskStandardCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask Grok (saves to history)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runGrokAsk(cmd, args, false) },
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
	for _, cmd := range []*cobra.Command{grokAskStandardCmd, grokAskIncognitoCmd} {
		cmd.Flags().StringVarP(&grokModel, "model", "m", "", "Model override (e.g. 'auto', 'fast', 'expert', 'thinking')")
		cmd.Flags().BoolVar(&grokDeepsearch, "deepsearch", false, "Enable DeepSearch mode")
		cmd.Flags().BoolVar(&grokReasoning, "reasoning", false, "Enable Reasoning mode")
	}
	grokAskStandardCmd.Flags().BoolVarP(&grokResume, "resume", "r", false, "Resume last conversation")
	grokAskStandardCmd.Flags().StringVar(&grokConversation, "conversation", "", "Continue a specific conversation by ID")
	grokCmd.AddCommand(grokAskStandardCmd)
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
		fmt.Fprintf(os.Stderr, "  webai-cli grok ask -c %s \"follow up\"\n", lastConvID)
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

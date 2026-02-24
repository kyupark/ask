package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qm4/webai-cli/internal/config"
	"github.com/qm4/webai-cli/internal/provider"
	chatgptpkg "github.com/qm4/webai-cli/internal/provider/chatgpt"
)

var (
	chatgptModel        string
	chatgptEffort       string
	chatgptResume       bool
	chatgptConversation string
)

var chatgptCmd = &cobra.Command{
	Use:   "chatgpt",
	Short: "ChatGPT commands",
	Long: `Interact with ChatGPT using browser cookies.
  ask            Ask a question (saves to history)
  ask-incognito  Ask a question (no history)
  list           List recent conversations
  models         Show available models`,
}

var chatgptAskStandardCmd = &cobra.Command{
	Use:   "ask [question]",
	Short: "Ask ChatGPT (saves to history)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runChatGPTAsk(cmd, args, false) },
}

var chatgptAskIncognitoCmd = &cobra.Command{
	Use:   "ask-incognito [question]",
	Short: "Ask ChatGPT (no history)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runChatGPTAsk(cmd, args, true) },
}

var chatgptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent ChatGPT conversations",
	Args:  cobra.NoArgs,
	RunE:  runChatGPTList,
}

var chatgptModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Show available ChatGPT models",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := chatgptpkg.New("", "", "", providerTimeout())
		return runModels(p)
	},
}

func init() {
	chatgptAskStandardCmd.Flags().StringVarP(&chatgptModel, "model", "m", "", "Model override (e.g. 'auto', 'gpt-5.2', 'gpt-5.2-thinking')")
	chatgptAskStandardCmd.Flags().StringVar(&chatgptEffort, "effort", "", "Thinking effort (none, low, medium, high, xhigh)")
	chatgptAskStandardCmd.Flags().BoolVarP(&chatgptResume, "resume", "r", false, "Resume last conversation")
	chatgptAskStandardCmd.Flags().StringVar(&chatgptConversation, "conversation", "", "Continue a specific conversation by ID")
	chatgptAskIncognitoCmd.Flags().StringVarP(&chatgptModel, "model", "m", "", "Model override (e.g. 'auto', 'gpt-5.2', 'gpt-5.2-thinking')")
	chatgptAskIncognitoCmd.Flags().StringVar(&chatgptEffort, "effort", "", "Thinking effort (none, low, medium, high, xhigh)")
	chatgptCmd.AddCommand(chatgptAskStandardCmd)
	chatgptCmd.AddCommand(chatgptAskIncognitoCmd)
	chatgptCmd.AddCommand(chatgptListCmd)
	chatgptCmd.AddCommand(chatgptModelsCmd)
	rootCmd.AddCommand(chatgptCmd)
}

func runChatGPTAsk(cmd *cobra.Command, args []string, temporary bool) error {
	query := strings.Join(args, " ")

	model := globalCfg.ChatGPT.Model
	if chatgptModel != "" {
		model = chatgptModel
	}

	p := chatgptpkg.New(
		globalCfg.ChatGPT.BaseURL,
		model,
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"__Secure-next-auth.session-token": globalCfg.ChatGPT.SessionToken,
		"cf_clearance":                     globalCfg.ChatGPT.CfClearance,
		"_puid":                            globalCfg.ChatGPT.PUID,
	})

	autoLoadCookies(cmd.Context(), p)

	// Apply thinking effort — skip default for debug.
	effort := chatgptEffort
	if effort == "" {
		effort = globalCfg.ChatGPT.Effort
	}
	if effort != "" {
		p.SetThinkingEffort(effort)
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
				fmt.Fprintf(os.Stderr, "[chatgpt] error: %v\n", err)
			}
		},
	}

	if !temporary {
		if chatgptConversation != "" {
			opts.ConversationID = chatgptConversation
		} else if chatgptResume {
			state := config.LoadState()
			if conv := state.GetConversation("chatgpt"); conv != nil {
				opts.ConversationID = conv.ConversationID
				opts.ParentMessageID = conv.ParentMessageID
			} else {
				fmt.Fprintln(os.Stderr, "No previous conversation found for chatgpt — starting new")
			}
		}
	}

	// Save conversation state and capture ID for hint.
	var lastConvID string
	if !temporary {
		opts.OnConversation = func(convID, parentMsgID, respID string) {
			lastConvID = convID
			state := config.LoadState()
			state.SetConversation("chatgpt", &config.ConversationState{
				ConversationID:  convID,
				ParentMessageID: parentMsgID,
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
		fmt.Fprintf(os.Stderr, "  chatmux chatgpt ask -c %s \"follow up\"\n", lastConvID)
	}

	return nil
}

func runChatGPTList(cmd *cobra.Command, args []string) error {
	p := chatgptpkg.New(
		globalCfg.ChatGPT.BaseURL,
		"",
		globalCfg.UserAgent,
		providerTimeout(),
	)

	p.SetCookies(map[string]string{
		"__Secure-next-auth.session-token": globalCfg.ChatGPT.SessionToken,
		"cf_clearance":                     globalCfg.ChatGPT.CfClearance,
		"_puid":                            globalCfg.ChatGPT.PUID,
	})

	return runList(cmd.Context(), p, 20)
}

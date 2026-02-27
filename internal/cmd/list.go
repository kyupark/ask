package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kyupark/ask/internal/provider"
)

// runList is a shared helper that lists conversations for any provider
// implementing the Lister interface.
func runList(ctx context.Context, p provider.Provider, limit int) error {
	lister, ok := p.(provider.Lister)
	if !ok {
		return fmt.Errorf("%s does not support listing conversations", p.Name())
	}

	autoLoadCookies(ctx, p)

	opts := provider.ListOptions{
		Limit:   limit,
		Verbose: globalCfg.Verbose,
	}
	if globalCfg.Verbose {
		opts.LogFunc = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	conversations, err := lister.ListConversations(ctx, opts)
	if err != nil {
		return err
	}

	if len(conversations) == 0 {
		fmt.Println("No conversations found.")
		return nil
	}

	fmt.Printf("Found %d conversation(s):\n\n", len(conversations))
	for _, c := range conversations {
		title := c.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("  %s\n", title)
		fmt.Printf("    ID: %s\n", c.ID)
		if !c.CreatedAt.IsZero() {
			fmt.Printf("    %s\n", formatTime(c.CreatedAt))
		}
		fmt.Println()
	}

	return nil
}

func formatTime(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() {
		return t.Format("Jan 2, 3:04 PM")
	}
	return t.Format("Jan 2, 2006, 3:04 PM")
}

// Package sse provides a shared Server-Sent Events parser used by
// providers that stream via SSE (Perplexity, ChatGPT).
package sse

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Event is a single SSE event with its data payload.
type Event struct {
	Data string
}

// Handler processes SSE events.
type Handler func(event Event) error

// Read reads SSE events from r and calls handler for each data line.
// Returns nil on normal completion (EOF) and an error only if the
// scanner or handler fails.
func Read(r io.Reader, handler Handler) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}

		if err := handler(Event{Data: data}); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading SSE stream: %w", err)
	}

	return nil
}

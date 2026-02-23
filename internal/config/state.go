package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const stateFile = "state.json"

// ConversationState holds continuation context for a single provider.
type ConversationState struct {
	ConversationID  string            `json:"conversation_id"`
	ParentMessageID string            `json:"parent_message_id,omitempty"`
	ResponseID      string            `json:"response_id,omitempty"`
	Extra           map[string]string `json:"extra,omitempty"`
}

type AskAllConversationState struct {
	Providers map[string]*ConversationState `json:"providers"`
}

// State holds runtime state persisted across CLI invocations.
type State struct {
	LastConversation map[string]*ConversationState       `json:"last_conversation"`
	AskAll           map[string]*AskAllConversationState `json:"ask_all,omitempty"`
	LastAskAllID     string                              `json:"last_ask_all_id,omitempty"`
}

// LoadState reads state from the XDG config directory, returning empty state if not found.
func LoadState() *State {
	s := &State{LastConversation: make(map[string]*ConversationState)}

	path := StatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}

	_ = json.Unmarshal(data, s)
	if s.LastConversation == nil {
		s.LastConversation = make(map[string]*ConversationState)
	}
	if s.AskAll == nil {
		s.AskAll = make(map[string]*AskAllConversationState)
	}

	return s
}

// SaveState writes state to the XDG config directory.
func SaveState(s *State) error {
	path := StatePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling state: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

// SetConversation stores conversation state for a provider.
func (s *State) SetConversation(provider string, cs *ConversationState) {
	if s.LastConversation == nil {
		s.LastConversation = make(map[string]*ConversationState)
	}
	s.LastConversation[provider] = cs
}

func (s *State) SetAskAllConversation(id string, providers map[string]*ConversationState) {
	if id == "" {
		return
	}
	if s.AskAll == nil {
		s.AskAll = make(map[string]*AskAllConversationState)
	}
	s.AskAll[id] = &AskAllConversationState{Providers: providers}
	s.LastAskAllID = id
}

func (s *State) GetAskAllConversation(id string) *AskAllConversationState {
	if s.AskAll == nil {
		return nil
	}
	return s.AskAll[id]
}

// GetConversation returns the last conversation state for a provider, or nil.
func (s *State) GetConversation(provider string) *ConversationState {
	if s.LastConversation == nil {
		return nil
	}
	return s.LastConversation[provider]
}

// StatePath returns the path to the state file.
func StatePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", appName, stateFile)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, appName, stateFile)
}

// Package state persists a small sidecar file recording what aiworker-cli
// itself has installed and where, so `remove`/`sync` know exactly which
// config keys and keychain entries belong to us without guessing from the
// client's config file (which may hold entries the user added by hand).
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Installed is one MCP server aiworker-cli wrote into a client config.
type Installed struct {
	ID          string    `json:"id"`
	Target      string    `json:"target"`      // "desktop" | "project"
	ConfigPath  string    `json:"config_path"` // absolute path to the client's config file
	EntryName   string    `json:"entry_name"`  // key used inside mcpServers
	InstalledAt time.Time `json:"installed_at"`
	SecretKeys  []string  `json:"secret_keys"` // env var names stored in the OS keychain
}

// State is the full sidecar file contents.
type State struct {
	MCP []Installed `json:"mcp"`

	// Skills marketplace config: a git remote shared skills live in.
	// SkillsRemoteAsked distinguishes "asked and left blank on purpose"
	// (stay local-only) from "never asked yet" (prompt once).
	SkillsRemote      string    `json:"skills_remote,omitempty"`
	SkillsRemoteAsked bool      `json:"skills_remote_asked,omitempty"`
	SkillsLastSync    time.Time `json:"skills_last_sync,omitempty"`
}

// Path returns ~/.aiworker-cli/state.json.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aiworker-cli", "state.json"), nil
}

// Load reads the sidecar file, returning an empty State if it doesn't exist yet.
func Load() (*State, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the sidecar file, creating ~/.aiworker-cli if needed.
func (s *State) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Find looks up an installed entry by catalog ID.
func (s *State) Find(id string) (Installed, bool) {
	for _, i := range s.MCP {
		if i.ID == id {
			return i, true
		}
	}
	return Installed{}, false
}

// Upsert adds or replaces the entry for the given catalog ID.
func (s *State) Upsert(inst Installed) {
	for i, existing := range s.MCP {
		if existing.ID == inst.ID {
			s.MCP[i] = inst
			return
		}
	}
	s.MCP = append(s.MCP, inst)
}

// Remove drops the entry for the given catalog ID, if present.
func (s *State) Remove(id string) {
	out := s.MCP[:0]
	for _, i := range s.MCP {
		if i.ID != id {
			out = append(out, i)
		}
	}
	s.MCP = out
}

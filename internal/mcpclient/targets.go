// Package mcpclient knows where each MCP-capable client keeps its config and
// how to install into it without disturbing anything else it holds.
//
// There are two genuinely different install mechanisms here, not one: the
// separate Claude Desktop app reads a plain JSON file we can safely merge
// into ourselves, while Claude Code (the terminal app) resolves MCP servers
// from its own internal state that we don't hand-edit - we shell out to the
// official `claude mcp` CLI for that instead (see internal/claudecode).
package mcpclient

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"aiworker/cli/internal/claudecode"
)

// Kind distinguishes how a Target is actually installed into.
type Kind int

const (
	// KindClaudeCode installs via `claude mcp add/remove` at Scope.
	KindClaudeCode Kind = iota
	// KindDesktopFile installs by merging into claude_desktop_config.json.
	KindDesktopFile
)

// Target is one place aiworker-cli can install an MCP server into. Every
// target here is machine-local (global to this PC, never a shared/checked-in
// config) - the team doesn't want MCP config shared via git.
type Target struct {
	ID          string
	Label       string
	Description string
	Kind        Kind
	Scope       claudecode.Scope       // meaningful for KindClaudeCode only
	pathFunc    func() (string, error) // meaningful for KindDesktopFile only
}

// Path resolves the config file path for a KindDesktopFile target.
func (t Target) Path() (string, error) {
	return t.pathFunc()
}

// Targets lists every supported install destination.
func Targets() []Target {
	return []Target{
		{
			ID:          "claude-user",
			Label:       "Claude Code (todos los proyectos)",
			Description: "claude mcp add --scope user: te lo reconoce Claude Code en cualquier carpeta de esta maquina",
			Kind:        KindClaudeCode,
			Scope:       claudecode.ScopeUser,
		},
		{
			ID:          "desktop",
			Label:       "Claude Desktop (app de escritorio)",
			Description: "Config de la app de escritorio separada (no la terminal) - claude_desktop_config.json",
			Kind:        KindDesktopFile,
			pathFunc:    desktopConfigPath,
		},
	}
}

// FindTarget looks up a target by ID.
func FindTarget(id string) (Target, bool) {
	for _, t := range Targets() {
		if t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}

func desktopConfigPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("no se encontro la variable de entorno APPDATA")
		}
		return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
	}
}

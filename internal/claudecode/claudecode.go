// Package claudecode wraps the official `claude mcp` CLI. Claude Code (the
// terminal app) resolves MCP servers from its own state in ~/.claude.json —
// a completely different place than the separate Claude Desktop app's
// claude_desktop_config.json. Rather than hand-editing that state ourselves
// (~/.claude.json is large, holds a lot beyond MCP config, and its exact
// schema isn't a stable contract), every write goes through `claude mcp
// add`/`claude mcp remove`/`claude mcp list` - the same commands a developer
// would type themselves, so approval prompts, health checks and scope
// handling all behave exactly as Claude Code expects, and status queries
// reflect the machine's actual live state instead of anything we cache.
package claudecode

import (
	"fmt"
	"os/exec"
	"strings"
)

// ScopeUser is the only scope aiworker-cli offers: global, every project on
// this machine. Project scope (.mcp.json, shared via git) is intentionally
// not exposed - the team doesn't want a shared, checked-in MCP config.
const ScopeUser Scope = "user"

// Scope mirrors `claude mcp add`'s --scope values.
type Scope string

// AddStdio registers a local-process server (command + args + env) under
// name, in scope. env holds only what should be readable in the config
// Claude Code stores - callers keep secret values out of it entirely (see
// internal/secretstore).
func AddStdio(name string, scope Scope, command string, args []string, env map[string]string) (output string, err error) {
	cmdArgs := []string{"mcp", "add", name, "-s", string(scope)}
	for k, v := range env {
		cmdArgs = append(cmdArgs, "-e", k+"="+v)
	}
	cmdArgs = append(cmdArgs, "--", command)
	cmdArgs = append(cmdArgs, args...)
	return run(cmdArgs...)
}

// AddHTTP registers a remote HTTP-transport server. Auth (if any) is OAuth,
// completed afterwards with `claude mcp login <name>` - never something this
// call handles.
func AddHTTP(name string, scope Scope, url string) (output string, err error) {
	return run("mcp", "add", name, "-s", string(scope), "--transport", "http", "--", url)
}

// Remove unregisters name. An empty scope removes it from whichever scope it
// is actually configured in (matches `claude mcp remove`'s own default).
func Remove(name string, scope Scope) (output string, err error) {
	args := []string{"mcp", "remove", name}
	if scope != "" {
		args = append(args, "-s", string(scope))
	}
	return run(args...)
}

func run(args ...string) (string, error) {
	out, err := exec.Command("claude", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("claude %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// Installed is one entry from `claude mcp list` - live state, not anything
// aiworker-cli tracks itself.
type Installed struct {
	Name   string
	Detail string // command/URL + status, as claude prints it - display only
}

// List asks Claude Code for every MCP server it currently has configured
// (any scope, any source - ours, another tool's, or added by hand). There is
// no machine-readable output for `claude mcp list` today, so this parses the
// same text a person would read; best-effort by design, callers should treat
// a parse miss as "couldn't confirm" rather than "definitely not there".
func List() ([]Installed, error) {
	out, err := exec.Command("claude", "mcp", "list").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("claude mcp list: %w", err)
	}
	return parseList(string(out)), nil
}

func parseList(out string) []Installed {
	var results []Installed
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		idx := strings.Index(line, ": ")
		if idx <= 0 {
			continue // banner/status lines ("Checking MCP server health…") have no ": "
		}
		name := strings.TrimSpace(line[:idx])
		if name == "" || strings.ContainsAny(name, " \t") {
			continue // not a "name: ..." row
		}
		results = append(results, Installed{Name: name, Detail: strings.TrimSpace(line[idx+2:])})
	}
	return results
}

// Package prereq checks, before showing the MCP servers screen, whether the
// runtime launchers the catalog actually needs (npx, uvx, ...) are on PATH —
// so a user without Node/uv installed gets a clear "falta esto, instalalo
// aca" instead of a silent failure the first time Claude tries to start the
// server. For tools with a well-known official installer (uv), it can also
// run that installer itself once the user confirms.
package prereq

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"aiworker/cli/internal/catalog"
)

// Installer describes how to fetch+install a missing tool automatically.
type Installer struct {
	// Describe explains what's about to run, shown before asking to confirm.
	Describe string
	// Command builds the platform-specific install command. Never nil when
	// an Installer is set.
	Command func() *exec.Cmd
	// FallbackPaths are absolute locations to check right after installing,
	// in case the current process's PATH is stale (common on Windows: PATH
	// changes need a fresh shell to take effect).
	FallbackPaths func() []string
}

// Tool is a known runtime launcher with a human label, an install hint, and
// optionally a way to auto-install it.
type Tool struct {
	Bin         string
	Label       string
	InstallHint string
	Installer   *Installer
}

var known = map[string]Tool{
	"npx": {
		Bin:         "npx",
		Label:       "Node.js / npx",
		InstallHint: "instalar Node.js 18+ (incluye npx): https://nodejs.org",
		// No single official cross-platform one-liner like uv's — installing
		// Node well (with a version manager, on every OS) is enough of a
		// judgment call that we point at the docs instead of guessing.
	},
	"uvx": {
		Bin:         "uvx",
		Label:       "uv / uvx",
		InstallHint: "instalar uv: https://docs.astral.sh/uv/getting-started/installation/",
		Installer: &Installer{
			Describe:      "Instala uv (astral.sh) via su script oficial de instalacion.",
			Command:       uvInstallCmd,
			FallbackPaths: uvFallbackPaths,
		},
	},
}

func uvInstallCmd() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			"irm https://astral.sh/uv/install.ps1 | iex")
	}
	return exec.Command("sh", "-c", "curl -LsSf https://astral.sh/uv/install.sh | sh")
}

func uvFallbackPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return []string{filepath.Join(home, ".local", "bin", "uvx.exe")}
	}
	return []string{
		filepath.Join(home, ".local", "bin", "uvx"),
		filepath.Join(home, ".cargo", "bin", "uvx"),
	}
}

// Result is the outcome of checking one tool.
type Result struct {
	Tool Tool
	OK   bool
	Path string
}

// CanAutoInstall reports whether this result's tool can be installed
// automatically (with confirmation) instead of just pointed at with a hint.
func (r Result) CanAutoInstall() bool {
	return r.Tool.Installer != nil
}

// RequiredBinaries returns the deduped set of launcher binaries the catalog
// needs, in catalog order.
func RequiredBinaries() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range catalog.MCPServers {
		if m.Command == "" || seen[m.Command] {
			continue
		}
		seen[m.Command] = true
		out = append(out, m.Command)
	}
	return out
}

// Check looks up a single binary on PATH.
func Check(bin string) Result {
	tool, ok := known[bin]
	if !ok {
		tool = Tool{Bin: bin, Label: bin}
	}
	path, err := exec.LookPath(bin)
	return Result{Tool: tool, OK: err == nil, Path: path}
}

// CheckAll checks every binary in bins, in order.
func CheckAll(bins []string) []Result {
	out := make([]Result, len(bins))
	for i, b := range bins {
		out[i] = Check(b)
	}
	return out
}

// AllOK reports whether every result passed.
func AllOK(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}

// Resolve looks up bin the normal way (PATH) and, if that fails and the tool
// has a known Installer, also checks its FallbackPaths — covering the case
// where we just installed it in this same run and the process's inherited
// PATH hasn't caught up yet.
func Resolve(bin string) (path string, ok bool) {
	if p, err := exec.LookPath(bin); err == nil {
		return p, true
	}
	tool, known := known[bin]
	if !known || tool.Installer == nil || tool.Installer.FallbackPaths == nil {
		return "", false
	}
	for _, p := range tool.Installer.FallbackPaths() {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

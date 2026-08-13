// Package doctor runs a handful of environment sanity checks so a new hire
// finds out immediately what's missing instead of hitting a cryptic MCP
// startup failure inside Claude later.
package doctor

import (
	"os/exec"

	"aiworker/cli/internal/prereq"
)

// Check is one pass/fail environment probe.
type Check struct {
	Label  string
	OK     bool
	Detail string
}

// Run executes every check and returns the results in a fixed, readable
// order: general dev tools first, then whatever runtime launchers the MCP
// catalog itself needs (npx, uvx, ...) — kept in sync with the catalog
// automatically instead of hardcoding a stale binary name here.
func Run() []Check {
	checks := []Check{
		checkBinary("git", "Git"),
		checkBinary("claude", "Claude Code CLI"),
	}
	for _, bin := range prereq.RequiredBinaries() {
		r := prereq.Check(bin)
		detail := r.Path
		if !r.OK {
			detail = r.Tool.InstallHint
		}
		checks = append(checks, Check{Label: r.Tool.Label, OK: r.OK, Detail: detail})
	}
	return checks
}

func checkBinary(bin, label string) Check {
	path, err := exec.LookPath(bin)
	if err != nil {
		return Check{Label: label, OK: false, Detail: bin + " no esta en el PATH"}
	}
	return Check{Label: label, OK: true, Detail: path}
}

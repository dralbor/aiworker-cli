// Package mcpinstall ties catalog + secretstore + mcpclient/claudecode +
// state together: it's the only place that knows how to turn a filled-in
// catalog entry into an actual installed MCP server (however that target
// installs things) and a keychain entry, and how to cleanly undo that.
//
// Install status is reconciled against Claude Code's own live registry
// (`claude mcp list`) every time it's asked for, not just our own sidecar
// bookkeeping - so a server installed by hand (or by anything else) shows up
// correctly, and a record we have for something that got removed outside
// aiworker-cli gets pruned instead of lying.
package mcpinstall

import (
	"fmt"
	"os"
	"time"

	"aiworker/cli/internal/catalog"
	"aiworker/cli/internal/claudecode"
	"aiworker/cli/internal/mcpclient"
	"aiworker/cli/internal/secretstore"
	"aiworker/cli/internal/state"
)

// EnvValue is one resolved (name, value) pair collected from the user for a
// given catalog.EnvVar.
type EnvValue struct {
	Name   string
	Value  string
	Secret bool
}

// Apply installs mcp into targetID (see mcpclient.Targets) under entryName,
// storing secret values in the OS keychain and everything else wherever the
// target actually keeps its config. It records what it did in the sidecar
// state file so Remove can undo exactly this and nothing else.
func Apply(mcp catalog.MCPServer, targetID, entryName string, values []EnvValue) error {
	target, ok := mcpclient.FindTarget(targetID)
	if !ok {
		return fmt.Errorf("destino desconocido: %s", targetID)
	}

	var configPath string
	var secretKeys []string

	switch mcp.Transport {
	case catalog.TransportHTTP:
		if target.Kind != mcpclient.KindClaudeCode {
			return fmt.Errorf("%s usa OAuth por HTTP, todavia solo se puede instalar en Claude Code", mcp.Name)
		}
		if out, err := claudecode.AddHTTP(entryName, target.Scope, mcp.URL); err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}

	default: // TransportStdio
		envMap := map[string]string{}
		for _, v := range values {
			if v.Secret {
				if err := secretstore.Set(mcp.ID, v.Name, v.Value); err != nil {
					return err
				}
				secretKeys = append(secretKeys, v.Name)
			} else if v.Value != "" {
				envMap[v.Name] = v.Value
			}
		}

		command, args := mcp.Command, mcp.Args
		if len(secretKeys) > 0 {
			// Secrets never touch any config file: point the client at our
			// own binary instead, which fetches them from the keychain and
			// execs the real server right before use. mcp-run re-resolves
			// mcp.Command/Args from the catalog by ID at launch time, so
			// nothing beyond the ID needs to travel in argv.
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("no pude resolver la ruta de aiworker-cli: %w", err)
			}
			command = self
			args = []string{"mcp-run", mcp.ID}
		}

		switch target.Kind {
		case mcpclient.KindClaudeCode:
			if out, err := claudecode.AddStdio(entryName, target.Scope, command, args, envMap); err != nil {
				return fmt.Errorf("%w: %s", err, out)
			}
		case mcpclient.KindDesktopFile:
			path, err := target.Path()
			if err != nil {
				return fmt.Errorf("resolviendo config de %s: %w", target.Label, err)
			}
			if err := mcpclient.Upsert(path, entryName, mcpclient.ServerEntry{Command: command, Args: args, Env: envMap}); err != nil {
				return err
			}
			configPath = path
		}
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	st.Upsert(state.Installed{
		ID:          mcp.ID,
		Target:      targetID,
		ConfigPath:  configPath,
		EntryName:   entryName,
		InstalledAt: time.Now(),
		SecretKeys:  secretKeys,
	})
	return st.Save()
}

// Remove undoes whatever Apply did for mcpID: deletes the config entry
// (wherever it actually lives), deletes any stored secrets, and drops the
// state record.
func Remove(mcpID string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	inst, ok := st.Find(mcpID)
	if !ok {
		return fmt.Errorf("%s no esta instalado (o no fue instalado por aiworker-cli)", mcpID)
	}

	target, ok := mcpclient.FindTarget(inst.Target)
	if !ok {
		return fmt.Errorf("%s se instalo en un destino que ya no existe (%s) - quitalo a mano", mcpID, inst.Target)
	}

	switch target.Kind {
	case mcpclient.KindClaudeCode:
		if out, err := claudecode.Remove(inst.EntryName, target.Scope); err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}
	case mcpclient.KindDesktopFile:
		if err := mcpclient.Remove(inst.ConfigPath, inst.EntryName); err != nil {
			return err
		}
	}

	for _, k := range inst.SecretKeys {
		if err := secretstore.Delete(mcpID, k); err != nil {
			return err
		}
	}

	st.Remove(mcpID)
	return st.Save()
}

// RemoveExternal removes a server aiworker-cli never installed (no state.json
// record, so no secrets or target details of ours to clean up) - just tells
// Claude Code to forget it, wherever it's configured.
func RemoveExternal(name string) error {
	out, err := claudecode.Remove(name, "")
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

// Status reports, for every catalog entry that's actually installed, whether
// aiworker-cli manages it (state.json has a record - full details, clean
// removal) or it was installed some other way (still shown as installed, but
// removal falls back to the generic `claude mcp remove`). Reconciled live
// against `claude mcp list` on every call: a state.json record for a
// Claude-Code-scoped entry that Claude Code no longer has gets pruned rather
// than reported as installed.
type Status struct {
	Installed   bool
	ManagedByUs bool
	Detail      state.Installed // valid only when ManagedByUs
}

func GetStatus() (map[string]Status, error) {
	st, err := state.Load()
	if err != nil {
		return nil, err
	}

	live, liveErr := claudecode.List()
	liveNames := map[string]bool{}
	for _, l := range live {
		liveNames[l.Name] = true
	}

	byID := map[string]state.Installed{}
	for _, i := range st.MCP {
		byID[i.ID] = i
	}

	out := map[string]Status{}
	dirty := false
	for _, mcp := range catalog.MCPServers {
		inState, managedByUs := byID[mcp.ID]

		if managedByUs && liveErr == nil {
			if target, ok := mcpclient.FindTarget(inState.Target); ok && target.Kind == mcpclient.KindClaudeCode && !liveNames[mcp.ID] {
				// We thought this was installed into Claude Code, but its
				// own registry disagrees now (removed outside aiworker-cli,
				// e.g. `claude mcp remove` run directly). Trust the live
				// check over our stale record.
				st.Remove(mcp.ID)
				dirty = true
				managedByUs = false
			}
		}

		installed := managedByUs || (liveErr == nil && liveNames[mcp.ID])
		if !installed {
			continue
		}
		out[mcp.ID] = Status{Installed: true, ManagedByUs: managedByUs, Detail: inState}
	}

	if dirty {
		_ = st.Save() // best-effort self-heal; a failed prune just means we ask again next time
	}
	return out, nil
}

// External lists MCP servers Claude Code has configured that aren't in our
// catalog at all - installed by hand, by another tool, or by a Claude Code
// plugin. Requires `claude` on PATH; returns an error if that fails so the
// caller can show "couldn't check" instead of silently claiming "none".
func External() ([]claudecode.Installed, error) {
	live, err := claudecode.List()
	if err != nil {
		return nil, err
	}
	catalogIDs := map[string]bool{}
	for _, m := range catalog.MCPServers {
		catalogIDs[m.ID] = true
	}
	var out []claudecode.Installed
	for _, l := range live {
		if !catalogIDs[l.Name] {
			out = append(out, l)
		}
	}
	return out, nil
}

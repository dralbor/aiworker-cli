// Package secretstore keeps MCP secret env vars (API tokens, etc.) out of
// client config files (Claude Desktop / Claude Code write plain JSON to
// disk) and out of process env vars set at config time. Values live in the
// OS-native credential store (Windows Credential Manager / macOS Keychain /
// Secret Service on Linux) and are only read back into memory right before
// exec'ing the real MCP server, by `aiworker mcp-run`.
package secretstore

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "aiworker-cli"

func account(mcpID, envName string) string {
	return mcpID + ":" + envName
}

// Set stores a secret value for a given MCP server + env var name.
func Set(mcpID, envName, value string) error {
	if err := keyring.Set(service, account(mcpID, envName), value); err != nil {
		return fmt.Errorf("guardando %s en el llavero del sistema: %w", envName, err)
	}
	return nil
}

// Get retrieves a previously stored secret value.
func Get(mcpID, envName string) (string, error) {
	val, err := keyring.Get(service, account(mcpID, envName))
	if err != nil {
		return "", fmt.Errorf("leyendo %s del llavero del sistema: %w", envName, err)
	}
	return val, nil
}

// Delete removes a stored secret. Missing entries are not an error.
func Delete(mcpID, envName string) error {
	err := keyring.Delete(service, account(mcpID, envName))
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("borrando %s del llavero del sistema: %w", envName, err)
	}
	return nil
}

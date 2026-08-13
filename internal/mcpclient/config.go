package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerEntry is the shape both Claude Desktop and Claude Code's .mcp.json
// expect under "mcpServers".
type ServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Upsert adds or replaces a single mcpServers entry in the config file at
// path, leaving every other top-level key (and every other server entry)
// untouched. The file and its parent directory are created if missing. A
// ".bak" copy of the previous contents is written before any change.
func Upsert(path, name string, entry ServerEntry) error {
	root, err := readRoot(path)
	if err != nil {
		return err
	}

	servers, err := readServers(root)
	if err != nil {
		return err
	}

	entryBytes, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	servers[name] = entryBytes

	return writeServers(path, root, servers)
}

// Remove deletes a single mcpServers entry, leaving everything else intact.
// It is not an error to remove an entry that is already absent.
func Remove(path, name string) error {
	root, err := readRoot(path)
	if err != nil {
		return err
	}

	servers, err := readServers(root)
	if err != nil {
		return err
	}
	delete(servers, name)

	return writeServers(path, root, servers)
}

// Has reports whether the config file at path currently has an entry named name.
func Has(path, name string) (bool, error) {
	root, err := readRoot(path)
	if err != nil {
		return false, err
	}
	servers, err := readServers(root)
	if err != nil {
		return false, err
	}
	_, ok := servers[name]
	return ok, nil
}

func readRoot(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", path, err)
	}
	if len(data) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s no es JSON valido, no lo voy a tocar: %w", path, err)
	}
	return root, nil
}

func readServers(root map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	servers := map[string]json.RawMessage{}
	if raw, ok := root["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, fmt.Errorf("la clave mcpServers existente no es un objeto valido: %w", err)
		}
	}
	return servers, nil
}

func writeServers(path string, root map[string]json.RawMessage, servers map[string]json.RawMessage) error {
	serversBytes, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	root["mcpServers"] = serversBytes

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creando carpeta para %s: %w", path, err)
	}

	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", existing, 0o600); err != nil {
			return fmt.Errorf("escribiendo backup de %s: %w", path, err)
		}
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("escribiendo %s: %w", path, err)
	}
	return nil
}

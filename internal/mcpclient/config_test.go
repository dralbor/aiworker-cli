package mcpclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertPreservesUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")

	initial := `{
  "someOtherSetting": true,
  "mcpServers": {
    "manual-entry": {"command": "manual", "args": []}
  }
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Upsert(path, "atlassian", ServerEntry{Command: "uvx", Args: []string{"mcp-atlassian@0.23.0"}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("resulting file is not valid JSON: %v", err)
	}

	if root["someOtherSetting"] != true {
		t.Errorf("unrelated top-level key was lost: %v", root)
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["manual-entry"]; !ok {
		t.Errorf("pre-existing manual mcpServers entry was lost: %v", servers)
	}
	if _, ok := servers["atlassian"]; !ok {
		t.Errorf("new entry was not written: %v", servers)
	}

	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("expected a .bak backup of the previous file: %v", err)
	}
}

func TestUpsertOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "claude_desktop_config.json")

	if err := Upsert(path, "omnisql", ServerEntry{Command: "omnisql-mcp"}); err != nil {
		t.Fatalf("Upsert on missing file: %v", err)
	}

	has, err := Has(path, "omnisql")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected omnisql entry to exist after Upsert")
	}
}

func TestRemoveIsNoopWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, "does-not-exist"); err != nil {
		t.Fatalf("Remove on absent entry should not error: %v", err)
	}
}

func TestUpsertRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(path, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Upsert(path, "omnisql", ServerEntry{Command: "omnisql-mcp"}); err == nil {
		t.Error("expected Upsert to refuse to touch a corrupt config file")
	}
}

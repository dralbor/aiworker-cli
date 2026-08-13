package claudecode

import "testing"

// Real output captured from `claude mcp list` while developing this package.
const sampleListOutput = "Checking MCP server health…\n\n" +
	"omnisql: omnisql-mcp  - ✔ Connected\n" +
	"aiworker-smoketest-http: https://mcp.figma.com/mcp (HTTP) - ! Needs authentication\n"

func TestParseList(t *testing.T) {
	got := parseList(sampleListOutput)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Name != "omnisql" {
		t.Errorf("entry 0 name = %q, want omnisql", got[0].Name)
	}
	if got[1].Name != "aiworker-smoketest-http" {
		t.Errorf("entry 1 name = %q, want aiworker-smoketest-http", got[1].Name)
	}
}

func TestParseListSkipsBannerAndBlankLines(t *testing.T) {
	got := parseList("Checking MCP server health…\n\n\n")
	if len(got) != 0 {
		t.Errorf("expected no entries from a banner-only listing, got %+v", got)
	}
}

func TestParseListHandlesColonsInName(t *testing.T) {
	// Plugin-scoped names can contain colons themselves (e.g.
	// "plugin:figma:figma") - the real separator is ": " (colon+space).
	got := parseList("plugin:figma:figma: https://mcp.figma.com/mcp (HTTP) - ! Needs authentication\n")
	if len(got) != 1 || got[0].Name != "plugin:figma:figma" {
		t.Fatalf("expected a single entry named plugin:figma:figma, got %+v", got)
	}
}

// Package catalog is the built-in registry of MCP servers this company's
// developers are expected to use. For the MVP it is a Go literal, mirroring
// how vulcan-cli hardcodes its tunnel targets — a separately-fetched catalog
// (git repo, S3 object) is the natural next step once this list outgrows a
// release cycle.
package catalog

// Transport is how a server is reached.
type Transport int

const (
	// TransportStdio launches Command locally (npx/uvx/a binary).
	TransportStdio Transport = iota
	// TransportHTTP talks to a remote URL; auth is OAuth via `claude mcp
	// login`, never an env var, so these entries never have Env.
	TransportHTTP
)

// EnvVar describes one environment variable an MCP server needs. Secret
// values are never written to the client's JSON config in plaintext — they
// go through internal/secretstore into the OS keychain instead.
type EnvVar struct {
	Name        string
	Description string
	Secret      bool
}

// MCPServer is one installable entry in the catalog. Descriptions here are
// deliberately short - what it does, one line. Version-pin rationale,
// security caveats etc. belong in README/commit messages, not the picker UI.
type MCPServer struct {
	ID          string // stable key: catalog id, Claude Code registered name, and keychain account prefix
	Name        string // display name
	Description string // one line: what it does
	Transport   Transport
	Command     string // TransportStdio only
	Args        []string
	URL         string // TransportHTTP only
	Env         []EnvVar
	DocsURL     string
}

// MCPServers is the full catalog, in display order.
var MCPServers = []MCPServer{
	{
		ID:          "omnisql",
		Name:        "OmniSQL (DBeaver)",
		Description: "Explora y consulta las conexiones de DBeaver (dev/qa/prod) desde Claude.",
		Transport:   TransportStdio,
		Command:     "npx",
		Args:        []string{"-y", "omnisql-mcp"},
		Env: []EnvVar{
			{Name: "OMNISQL_ALLOWED_CONNECTIONS", Description: "Conexiones DBeaver habilitadas (opcional)"},
		},
		DocsURL: "https://github.com/srthkdev/omnisql-mcp",
	},
	{
		ID:          "atlassian",
		Name:        "Atlassian (Jira / Confluence)",
		Description: "Buscar, crear y actualizar issues de Jira y paginas de Confluence desde Claude.",
		Transport:   TransportStdio,
		Command:     "uvx",
		Args:        []string{"mcp-atlassian"},
		Env: []EnvVar{
			{Name: "JIRA_URL", Description: "https://tuempresa.atlassian.net"},
			{Name: "JIRA_USERNAME", Description: "tu email de Atlassian"},
			{Name: "JIRA_API_TOKEN", Description: "id.atlassian.com/manage-profile/security/api-tokens", Secret: true},
			{Name: "CONFLUENCE_URL", Description: "https://tuempresa.atlassian.net/wiki"},
			{Name: "CONFLUENCE_USERNAME", Description: "tu email de Atlassian"},
			{Name: "CONFLUENCE_API_TOKEN", Description: "mismo token o uno separado", Secret: true},
		},
		DocsURL: "https://github.com/sooperset/mcp-atlassian",
	},
	{
		ID:          "figma",
		Name:        "Figma",
		Description: "Leer archivos, componentes y estilos de Figma desde Claude. Pide iniciar sesion con tu cuenta de Figma (SSO/OAuth) la primera vez.",
		Transport:   TransportHTTP,
		URL:         "https://mcp.figma.com/mcp",
		DocsURL:     "https://developers.figma.com/docs/figma-mcp-server/",
	},
}

// Find returns the catalog entry with the given ID, or false if unknown.
func Find(id string) (MCPServer, bool) {
	for _, m := range MCPServers {
		if m.ID == id {
			return m, true
		}
	}
	return MCPServer{}, false
}

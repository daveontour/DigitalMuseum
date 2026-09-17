package model

// MCPServerRow is one row in the deployment-wide mcp_servers table — an MCP server the app
// discovers AI tools from. Exactly one row has IsBuiltin true: the bundled cmd/mcpserver, whose
// EndpointURL/AuthToken are unused (its real connection is Electron-managed).
type MCPServerRow struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	EndpointURL string `json:"endpoint_url"`
	AuthToken   string `json:"auth_token"`
	Enabled     bool   `json:"enabled"`
	IsBuiltin   bool   `json:"is_builtin"`
	SortOrder   int    `json:"sort_order"`
}

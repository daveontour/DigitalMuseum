package model

// MCPServerRow is one row in the deployment-wide mcp_servers table — an MCP server the app
// discovers AI tools from. Exactly two rows have IsBuiltin true: the bundled cmd/mcpserver
// (Scope "archive") and the bundled cmd/chatbotmcpserver (Scope "chatbot"); both have unused
// EndpointURL/AuthToken (their real connections are Electron-managed). Every other row is
// owner-added, Scope "shared", usable by both the persona chat and the ChatBot.
type MCPServerRow struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	EndpointURL string `json:"endpoint_url"`
	AuthToken   string `json:"auth_token"`
	Enabled     bool   `json:"enabled"`
	IsBuiltin   bool   `json:"is_builtin"`
	SortOrder   int    `json:"sort_order"`
	Scope       string `json:"scope"`
}

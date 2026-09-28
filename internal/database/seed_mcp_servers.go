package database

import (
	"context"
	"database/sql"
	"fmt"
)

// BuiltinMCPServerName is the reserved display name for the bundled archive-data MCP tools
// server (cmd/mcpserver) row in mcp_servers. Its endpoint_url/auth_token columns are unused — the
// real connection is Electron-managed (MCP_SERVER_URL/MCP_AUTH_TOKEN, see
// internal/ai.DefaultMCPClient) — only its enabled flag is meaningful. See
// internal/service/mcp_servers_service.go.
const BuiltinMCPServerName = "Digital Museum"

// BuiltinChatbotMCPServerName is the reserved display name for the bundled ChatBot-only
// utility-tools MCP server (cmd/chatbotmcpserver) row in mcp_servers. Like the archive server,
// its endpoint_url/auth_token columns are unused — Electron manages the real connection via
// CHATBOT_MCP_SERVER_URL/MCP_AUTH_TOKEN (see internal/ai.DefaultChatbotMCPClient).
const BuiltinChatbotMCPServerName = "ChatBot Tools"

// SeedBuiltinMCPServerIfMissing ensures the mcp_servers row for the bundled archive-data MCP
// tools server exists, inserting it once (enabled, sort_order -1 so it's listed first) if
// absent. Idempotent — safe to call on every startup, same insert-if-missing pattern as
// SeedAIModelsFromFileIfMissing. Filters by scope='archive', not just is_builtin=1, since a
// second is_builtin row (the ChatBot Tools server, scope='chatbot') now also exists —
// SeedBuiltinChatbotMCPServerIfMissing follows the same discipline for its own scope.
func SeedBuiltinMCPServerIfMissing(ctx context.Context, db *sql.DB) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM mcp_servers WHERE is_builtin = 1 AND scope = 'archive'`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check builtin mcp server: %w", err)
	}
	if exists > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO mcp_servers (name, endpoint_url, auth_token, enabled, is_builtin, sort_order, scope)
		 VALUES (?, '', NULL, 1, 1, -1, 'archive')`,
		BuiltinMCPServerName)
	if err != nil {
		return fmt.Errorf("insert builtin mcp server: %w", err)
	}
	return nil
}

// SeedBuiltinChatbotMCPServerIfMissing ensures the mcp_servers row for the bundled ChatBot
// utility-tools server exists, inserting it once (enabled, sort_order -2 so it sorts just above
// the archive server) if absent. Idempotent, same pattern as SeedBuiltinMCPServerIfMissing.
func SeedBuiltinChatbotMCPServerIfMissing(ctx context.Context, db *sql.DB) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM mcp_servers WHERE is_builtin = 1 AND scope = 'chatbot'`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check builtin chatbot mcp server: %w", err)
	}
	if exists > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO mcp_servers (name, endpoint_url, auth_token, enabled, is_builtin, sort_order, scope)
		 VALUES (?, '', NULL, 1, 1, -2, 'chatbot')`,
		BuiltinChatbotMCPServerName)
	if err != nil {
		return fmt.Errorf("insert builtin chatbot mcp server: %w", err)
	}
	return nil
}

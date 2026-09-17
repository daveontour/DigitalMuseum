package database

import (
	"context"
	"database/sql"
	"fmt"
)

// BuiltinMCPServerName is the reserved display name for the bundled MCP tools server
// (cmd/mcpserver) row in mcp_servers. Its endpoint_url/auth_token columns are unused — the real
// connection is Electron-managed (MCP_SERVER_URL/MCP_AUTH_TOKEN, see internal/ai.DefaultMCPClient)
// — only its enabled flag is meaningful. See internal/service/mcp_servers_service.go.
const BuiltinMCPServerName = "Digital Museum"

// SeedBuiltinMCPServerIfMissing ensures the mcp_servers row for the bundled MCP tools server
// exists, inserting it once (enabled, sort_order -1 so it's listed first) if absent. Idempotent —
// safe to call on every startup, same insert-if-missing pattern as SeedAIModelsFromFileIfMissing.
func SeedBuiltinMCPServerIfMissing(ctx context.Context, db *sql.DB) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM mcp_servers WHERE is_builtin = 1`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check builtin mcp server: %w", err)
	}
	if exists > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO mcp_servers (name, endpoint_url, auth_token, enabled, is_builtin, sort_order)
		 VALUES (?, '', NULL, 1, 1, -1)`,
		BuiltinMCPServerName)
	if err != nil {
		return fmt.Errorf("insert builtin mcp server: %w", err)
	}
	return nil
}

package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// MCPServersRepo accesses the deployment-wide mcp_servers table (no user_id scoping — same
// rationale as AIModelsRepo: each archive is already siloed at the SQLite-file level).
type MCPServersRepo struct {
	pool *sql.DB
}

// NewMCPServersRepo creates an MCPServersRepo.
func NewMCPServersRepo(pool *sql.DB) *MCPServersRepo {
	return &MCPServersRepo{pool: pool}
}

const mcpServerColumns = `id, name, endpoint_url, auth_token, enabled, is_builtin, sort_order`

func scanMCPServerRow(row interface{ Scan(...any) error }) (*model.MCPServerRow, error) {
	var r model.MCPServerRow
	var authToken sql.NullString
	if err := row.Scan(&r.ID, &r.Name, &r.EndpointURL, &authToken, &r.Enabled, &r.IsBuiltin, &r.SortOrder); err != nil {
		return nil, err
	}
	r.AuthToken = authToken.String
	return &r, nil
}

// ListAll returns every mcp_servers row ordered by sort_order, id.
func (r *MCPServersRepo) ListAll(ctx context.Context) ([]*model.MCPServerRow, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT `+mcpServerColumns+` FROM mcp_servers ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("listAll mcp_servers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.MCPServerRow
	for rows.Next() {
		row, err := scanMCPServerRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID returns one mcp_servers row or nil when missing.
func (r *MCPServersRepo) GetByID(ctx context.Context, id int64) (*model.MCPServerRow, error) {
	row, err := scanMCPServerRow(r.pool.QueryRowContext(ctx,
		`SELECT `+mcpServerColumns+` FROM mcp_servers WHERE id = ?`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// NameExistsExcluding reports whether another row uses name, excluding excludeID (pass 0 to
// check against every row, e.g. on Create).
func (r *MCPServersRepo) NameExistsExcluding(ctx context.Context, name string, excludeID int64) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM mcp_servers WHERE name = ? AND id != ?`, name, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("nameExistsExcluding mcp_servers: %w", err)
	}
	return n > 0, nil
}

// Create inserts a new (always non-builtin — see MCPServersService) mcp_servers row and returns it.
func (r *MCPServersRepo) Create(ctx context.Context, name, endpointURL, authToken string, enabled bool, sortOrder int) (*model.MCPServerRow, error) {
	res, err := r.pool.ExecContext(ctx,
		`INSERT INTO mcp_servers (name, endpoint_url, auth_token, enabled, is_builtin, sort_order)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		name, endpointURL, nullableString(authToken), enabled, sortOrder)
	if err != nil {
		return nil, fmt.Errorf("create mcp_server: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create mcp_server last insert id: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Update replaces the editable fields of an existing row. Callers (MCPServersService) are
// responsible for refusing to change name/endpoint_url/auth_token on the builtin row — this
// layer applies whatever it's given.
func (r *MCPServersRepo) Update(ctx context.Context, id int64, name, endpointURL, authToken string, enabled bool, sortOrder int) (*model.MCPServerRow, error) {
	res, err := r.pool.ExecContext(ctx,
		`UPDATE mcp_servers SET name = ?, endpoint_url = ?, auth_token = ?, enabled = ?, sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		name, endpointURL, nullableString(authToken), enabled, sortOrder, id)
	if err != nil {
		return nil, fmt.Errorf("update mcp_server: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id)
}

// Delete removes a mcp_servers row by id. Callers are responsible for refusing to delete the
// builtin row.
func (r *MCPServersRepo) Delete(ctx context.Context, id int64) (bool, error) {
	res, err := r.pool.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete mcp_server: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

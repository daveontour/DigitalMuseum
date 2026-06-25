package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// RegionsRepo accesses the deployment-wide regions table (no user_id scoping).
type RegionsRepo struct {
	pool *sql.DB
}

// NewRegionsRepo creates a RegionsRepo.
func NewRegionsRepo(pool *sql.DB) *RegionsRepo {
	return &RegionsRepo{pool: pool}
}

func scanRegionRow(row interface{ Scan(...any) error }) (*model.RegionRow, error) {
	var r model.RegionRow
	if err := row.Scan(&r.ID, &r.Key, &r.SortOrder, &r.Text); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListAll returns all region rows ordered for display and bbox matching.
func (r *RegionsRepo) ListAll(ctx context.Context) ([]*model.RegionRow, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, key, sort_order, text FROM regions ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("listAll regions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.RegionRow
	for rows.Next() {
		row, err := scanRegionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID returns one region row or nil when missing.
func (r *RegionsRepo) GetByID(ctx context.Context, id int64) (*model.RegionRow, error) {
	row, err := scanRegionRow(r.pool.QueryRowContext(ctx,
		`SELECT id, key, sort_order, text FROM regions WHERE id = ?`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// GetByKey returns one region row or nil when missing.
func (r *RegionsRepo) GetByKey(ctx context.Context, key string) (*model.RegionRow, error) {
	row, err := scanRegionRow(r.pool.QueryRowContext(ctx,
		`SELECT id, key, sort_order, text FROM regions WHERE key = ?`, key))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// KeyExists reports whether a region key is already stored.
func (r *RegionsRepo) KeyExists(ctx context.Context, key string) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx, `SELECT COUNT(1) FROM regions WHERE key = ?`, key).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("keyExists regions: %w", err)
	}
	return n > 0, nil
}

// KeyExistsExcluding reports whether another row uses key, excluding excludeID (0 = none).
func (r *RegionsRepo) KeyExistsExcluding(ctx context.Context, key string, excludeID int64) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM regions WHERE key = ? AND id != ?`, key, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("keyExistsExcluding regions: %w", err)
	}
	return n > 0, nil
}

// Create inserts a new region row and returns it.
func (r *RegionsRepo) Create(ctx context.Context, key string, sortOrder int, text string) (*model.RegionRow, error) {
	res, err := r.pool.ExecContext(ctx,
		`INSERT INTO regions (key, sort_order, text) VALUES (?, ?, ?)`,
		key, sortOrder, text)
	if err != nil {
		return nil, fmt.Errorf("create region: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create region last insert id: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Update replaces key, sort_order, and text for an existing row.
func (r *RegionsRepo) Update(ctx context.Context, id int64, key string, sortOrder int, text string) (*model.RegionRow, error) {
	res, err := r.pool.ExecContext(ctx,
		`UPDATE regions SET key = ?, sort_order = ?, text = ? WHERE id = ?`,
		key, sortOrder, text, id)
	if err != nil {
		return nil, fmt.Errorf("update region: %w", err)
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

// Delete removes a region row by id.
func (r *RegionsRepo) Delete(ctx context.Context, id int64) (bool, error) {
	res, err := r.pool.ExecContext(ctx, `DELETE FROM regions WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete region: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ReorderSortOrder updates sort_order for each item.
func (r *RegionsRepo) ReorderSortOrder(ctx context.Context, items []struct {
	ID        int64
	SortOrder int
}) error {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reorder regions begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range items {
		if _, err := tx.ExecContext(ctx,
			`UPDATE regions SET sort_order = ? WHERE id = ?`, item.SortOrder, item.ID); err != nil {
			return fmt.Errorf("reorder regions id=%d: %w", item.ID, err)
		}
	}
	return tx.Commit()
}

// MaxSortOrder returns the highest sort_order value, or -1 when empty.
func (r *RegionsRepo) MaxSortOrder(ctx context.Context) (int, error) {
	var max sql.NullInt64
	err := r.pool.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM regions`).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("maxSortOrder regions: %w", err)
	}
	if !max.Valid {
		return -1, nil
	}
	return int(max.Int64), nil
}

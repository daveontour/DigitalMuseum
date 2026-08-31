package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// AIModelsRepo accesses the deployment-wide ai_models table (no user_id scoping).
type AIModelsRepo struct {
	pool *sql.DB
}

// NewAIModelsRepo creates an AIModelsRepo.
func NewAIModelsRepo(pool *sql.DB) *AIModelsRepo {
	return &AIModelsRepo{pool: pool}
}

const aiModelColumns = `id, key, display_name, model_slug, enabled, sort_order`

func scanAIModelRow(row interface{ Scan(...any) error }) (*model.AIModelRow, error) {
	var r model.AIModelRow
	if err := row.Scan(&r.ID, &r.Key, &r.DisplayName, &r.ModelSlug, &r.Enabled, &r.SortOrder); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListAll returns every ai_models row ordered by sort_order, id.
func (r *AIModelsRepo) ListAll(ctx context.Context) ([]*model.AIModelRow, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT `+aiModelColumns+` FROM ai_models ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("listAll ai_models: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.AIModelRow
	for rows.Next() {
		row, err := scanAIModelRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID returns one ai_models row or nil when missing.
func (r *AIModelsRepo) GetByID(ctx context.Context, id int64) (*model.AIModelRow, error) {
	row, err := scanAIModelRow(r.pool.QueryRowContext(ctx,
		`SELECT `+aiModelColumns+` FROM ai_models WHERE id = ?`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// GetByKey returns one ai_models row or nil when missing.
func (r *AIModelsRepo) GetByKey(ctx context.Context, key string) (*model.AIModelRow, error) {
	row, err := scanAIModelRow(r.pool.QueryRowContext(ctx,
		`SELECT `+aiModelColumns+` FROM ai_models WHERE key = ?`, key))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// KeyExists reports whether an ai_models key is already stored.
func (r *AIModelsRepo) KeyExists(ctx context.Context, key string) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx, `SELECT COUNT(1) FROM ai_models WHERE key = ?`, key).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("keyExists ai_models: %w", err)
	}
	return n > 0, nil
}

// KeyExistsExcluding reports whether another row uses key, excluding excludeID.
func (r *AIModelsRepo) KeyExistsExcluding(ctx context.Context, key string, excludeID int64) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM ai_models WHERE key = ? AND id != ?`, key, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("keyExistsExcluding ai_models: %w", err)
	}
	return n > 0, nil
}

// Create inserts a new ai_models row and returns it.
func (r *AIModelsRepo) Create(ctx context.Context, key, displayName, modelSlug string, enabled bool, sortOrder int) (*model.AIModelRow, error) {
	res, err := r.pool.ExecContext(ctx,
		`INSERT INTO ai_models (key, display_name, model_slug, enabled, sort_order) VALUES (?, ?, ?, ?, ?)`,
		key, displayName, modelSlug, enabled, sortOrder)
	if err != nil {
		return nil, fmt.Errorf("create ai_model: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create ai_model last insert id: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Update replaces the editable fields of an existing row.
func (r *AIModelsRepo) Update(ctx context.Context, id int64, key, displayName, modelSlug string, enabled bool, sortOrder int) (*model.AIModelRow, error) {
	res, err := r.pool.ExecContext(ctx,
		`UPDATE ai_models SET key = ?, display_name = ?, model_slug = ?, enabled = ?, sort_order = ? WHERE id = ?`,
		key, displayName, modelSlug, enabled, sortOrder, id)
	if err != nil {
		return nil, fmt.Errorf("update ai_model: %w", err)
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

// Delete removes an ai_models row by id.
func (r *AIModelsRepo) Delete(ctx context.Context, id int64) (bool, error) {
	res, err := r.pool.ExecContext(ctx, `DELETE FROM ai_models WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete ai_model: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// GuideTopicsRepo accesses the deployment-wide guide_topics table (no user_id scoping).
type GuideTopicsRepo struct {
	pool *sql.DB
}

// NewGuideTopicsRepo creates a GuideTopicsRepo.
func NewGuideTopicsRepo(pool *sql.DB) *GuideTopicsRepo {
	return &GuideTopicsRepo{pool: pool}
}

func scanGuideTopicRow(row interface{ Scan(...any) error }) (*model.GuideTopicRow, error) {
	var r model.GuideTopicRow
	if err := row.Scan(&r.ID, &r.Key, &r.Text); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListAll returns all guide topic rows ordered by id.
func (r *GuideTopicsRepo) ListAll(ctx context.Context) ([]*model.GuideTopicRow, error) {
	rows, err := r.pool.QueryContext(ctx, `SELECT id, key, text FROM guide_topics ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("ListAll guide_topics: %w", err)
	}
	defer rows.Close()
	var out []*model.GuideTopicRow
	for rows.Next() {
		row, err := scanGuideTopicRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID returns one guide topic row or nil when missing.
func (r *GuideTopicsRepo) GetByID(ctx context.Context, id int64) (*model.GuideTopicRow, error) {
	row, err := scanGuideTopicRow(r.pool.QueryRowContext(ctx,
		`SELECT id, key, text FROM guide_topics WHERE id = ?`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// GetByKey returns one guide topic row or nil when missing.
func (r *GuideTopicsRepo) GetByKey(ctx context.Context, key string) (*model.GuideTopicRow, error) {
	row, err := scanGuideTopicRow(r.pool.QueryRowContext(ctx,
		`SELECT id, key, text FROM guide_topics WHERE key = ?`, key))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// KeyExists reports whether a guide topic key is already stored.
func (r *GuideTopicsRepo) KeyExists(ctx context.Context, key string) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx, `SELECT COUNT(1) FROM guide_topics WHERE key = ?`, key).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("KeyExists guide_topics: %w", err)
	}
	return n > 0, nil
}

// KeyExistsExcluding reports whether another row uses key, excluding excludeID (0 = none).
func (r *GuideTopicsRepo) KeyExistsExcluding(ctx context.Context, key string, excludeID int64) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM guide_topics WHERE key = ? AND id != ?`, key, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("KeyExistsExcluding guide_topics: %w", err)
	}
	return n > 0, nil
}

// Create inserts a new guide topic row and returns it.
func (r *GuideTopicsRepo) Create(ctx context.Context, key, text string) (*model.GuideTopicRow, error) {
	res, err := r.pool.ExecContext(ctx, `INSERT INTO guide_topics (key, text) VALUES (?, ?)`, key, text)
	if err != nil {
		return nil, fmt.Errorf("Create guide_topic: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("Create guide_topic last insert id: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Update replaces key and text for an existing row.
func (r *GuideTopicsRepo) Update(ctx context.Context, id int64, key, text string) (*model.GuideTopicRow, error) {
	res, err := r.pool.ExecContext(ctx, `UPDATE guide_topics SET key = ?, text = ? WHERE id = ?`, key, text, id)
	if err != nil {
		return nil, fmt.Errorf("Update guide_topic: %w", err)
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

// Delete removes a guide topic row by id.
func (r *GuideTopicsRepo) Delete(ctx context.Context, id int64) (bool, error) {
	res, err := r.pool.ExecContext(ctx, `DELETE FROM guide_topics WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("Delete guide_topic: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

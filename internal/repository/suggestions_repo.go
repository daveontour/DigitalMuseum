package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// SuggestionsRepo accesses the deployment-wide suggestions table (no user_id scoping).
type SuggestionsRepo struct {
	pool *sql.DB
}

// NewSuggestionsRepo creates a SuggestionsRepo.
func NewSuggestionsRepo(pool *sql.DB) *SuggestionsRepo {
	return &SuggestionsRepo{pool: pool}
}

func scanSuggestionRow(row interface{ Scan(...any) error }) (*model.SuggestionRow, error) {
	var s model.SuggestionRow
	if err := row.Scan(&s.ID, &s.Key, &s.Text); err != nil {
		return nil, err
	}
	return &s, nil
}

// ListAll returns all suggestion rows ordered by id.
func (r *SuggestionsRepo) ListAll(ctx context.Context) ([]*model.SuggestionRow, error) {
	rows, err := r.pool.QueryContext(ctx, `SELECT id, key, text FROM suggestions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listAll suggestions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.SuggestionRow
	for rows.Next() {
		row, err := scanSuggestionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID returns one suggestion row or nil when missing.
func (r *SuggestionsRepo) GetByID(ctx context.Context, id int64) (*model.SuggestionRow, error) {
	row, err := scanSuggestionRow(r.pool.QueryRowContext(ctx,
		`SELECT id, key, text FROM suggestions WHERE id = ?`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// GetByKey returns one suggestion row or nil when missing.
func (r *SuggestionsRepo) GetByKey(ctx context.Context, key string) (*model.SuggestionRow, error) {
	row, err := scanSuggestionRow(r.pool.QueryRowContext(ctx,
		`SELECT id, key, text FROM suggestions WHERE key = ?`, key))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// KeyExists reports whether a suggestion key is already stored.
func (r *SuggestionsRepo) KeyExists(ctx context.Context, key string) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx, `SELECT COUNT(1) FROM suggestions WHERE key = ?`, key).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("keyExists suggestions: %w", err)
	}
	return n > 0, nil
}

// KeyExistsExcluding reports whether another row uses key, excluding excludeID (0 = none).
func (r *SuggestionsRepo) KeyExistsExcluding(ctx context.Context, key string, excludeID int64) (bool, error) {
	var n int
	err := r.pool.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM suggestions WHERE key = ? AND id != ?`, key, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("keyExistsExcluding suggestions: %w", err)
	}
	return n > 0, nil
}

// Create inserts a new suggestion row and returns it.
func (r *SuggestionsRepo) Create(ctx context.Context, key, text string) (*model.SuggestionRow, error) {
	res, err := r.pool.ExecContext(ctx, `INSERT INTO suggestions (key, text) VALUES (?, ?)`, key, text)
	if err != nil {
		return nil, fmt.Errorf("create suggestion: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create suggestion last insert id: %w", err)
	}
	return r.GetByID(ctx, id)
}

// Update replaces key and text for an existing row.
func (r *SuggestionsRepo) Update(ctx context.Context, id int64, key, text string) (*model.SuggestionRow, error) {
	res, err := r.pool.ExecContext(ctx, `UPDATE suggestions SET key = ?, text = ? WHERE id = ?`, key, text, id)
	if err != nil {
		return nil, fmt.Errorf("update suggestion: %w", err)
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

// Delete removes a suggestion row by id.
func (r *SuggestionsRepo) Delete(ctx context.Context, id int64) (bool, error) {
	res, err := r.pool.ExecContext(ctx, `DELETE FROM suggestions WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete suggestion: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// SavedResponseRepo accesses the saved_responses table.
type SavedResponseRepo struct {
	pool *sql.DB
}

// NewSavedResponseRepo creates a SavedResponseRepo.
func NewSavedResponseRepo(pool *sql.DB) *SavedResponseRepo {
	return &SavedResponseRepo{pool: pool}
}

func scanSavedResponse(row interface{ Scan(...any) error }) (*model.SavedResponse, error) {
	var s model.SavedResponse
	err := row.Scan(&s.ID, &s.Title, &s.Content, &s.Voice, &s.LLMProvider, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns all saved responses newest first.
func (r *SavedResponseRepo) List(ctx context.Context) ([]*model.SavedResponse, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, title, content, voice, llm_provider, created_at
	      FROM saved_responses WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY created_at DESC"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listSavedResponses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.SavedResponse
	for rows.Next() {
		s, err := scanSavedResponse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetByID returns a single saved response.
func (r *SavedResponseRepo) GetByID(ctx context.Context, id int64) (*model.SavedResponse, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, title, content, voice, llm_provider, created_at
	      FROM saved_responses WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	s, err := scanSavedResponse(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// Create inserts a new saved response.
func (r *SavedResponseRepo) Create(ctx context.Context, title, content string, voice, llmProvider *string) (*model.SavedResponse, error) {
	uid := uidFromCtx(ctx)
	s, err := scanSavedResponse(r.pool.QueryRowContext(ctx,
		`INSERT INTO saved_responses (title, content, voice, llm_provider, user_id)
		 VALUES (?1,?2,?3,?4,?5)
		 RETURNING id, title, content, voice, llm_provider, created_at`,
		title, content, voice, llmProvider, uidVal(uid),
	))
	if err != nil {
		return nil, fmt.Errorf("createSavedResponse: %w", err)
	}
	return s, nil
}

// Update modifies a saved response.
func (r *SavedResponseRepo) Update(ctx context.Context, id int64, title, content, voice, llmProvider *string) (*model.SavedResponse, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE saved_responses SET
	      title        = COALESCE(?1, title),
	      content      = COALESCE(?2, content),
	      voice        = COALESCE(?3, voice),
	      llm_provider = COALESCE(?4, llm_provider)
	      WHERE id = ?5`
	args := []any{title, content, voice, llmProvider, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING id, title, content, voice, llm_provider, created_at`
	s, err := scanSavedResponse(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("updateSavedResponse %d: %w", id, err)
	}
	return s, nil
}

// Delete removes a saved response. Returns false if not found.
func (r *SavedResponseRepo) Delete(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM saved_responses WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return rowsAffectedOrZero(tag) > 0, nil
}

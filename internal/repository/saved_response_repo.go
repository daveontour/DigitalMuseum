package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SavedResponseRepo accesses the saved_responses table.
type SavedResponseRepo struct {
	pool *pgxpool.Pool
}

// NewSavedResponseRepo creates a SavedResponseRepo.
func NewSavedResponseRepo(pool *pgxpool.Pool) *SavedResponseRepo {
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
	rows, err := r.pool.Query(ctx,
		`SELECT id, title, content, voice, llm_provider, created_at
		 FROM saved_responses ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("ListSavedResponses: %w", err)
	}
	defer rows.Close()
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
	s, err := scanSavedResponse(r.pool.QueryRow(ctx,
		`SELECT id, title, content, voice, llm_provider, created_at
		 FROM saved_responses WHERE id = $1`, id))
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
	s, err := scanSavedResponse(r.pool.QueryRow(ctx,
		`INSERT INTO saved_responses (title, content, voice, llm_provider)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, title, content, voice, llm_provider, created_at`,
		title, content, voice, llmProvider,
	))
	if err != nil {
		return nil, fmt.Errorf("CreateSavedResponse: %w", err)
	}
	return s, nil
}

// Update modifies a saved response.
func (r *SavedResponseRepo) Update(ctx context.Context, id int64, title, content, voice, llmProvider *string) (*model.SavedResponse, error) {
	s, err := scanSavedResponse(r.pool.QueryRow(ctx,
		`UPDATE saved_responses SET
		 title        = COALESCE($1, title),
		 content      = COALESCE($2, content),
		 voice        = COALESCE($3, voice),
		 llm_provider = COALESCE($4, llm_provider)
		 WHERE id = $5
		 RETURNING id, title, content, voice, llm_provider, created_at`,
		title, content, voice, llmProvider, id,
	))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("UpdateSavedResponse %d: %w", id, err)
	}
	return s, nil
}

// Delete removes a saved response. Returns false if not found.
func (r *SavedResponseRepo) Delete(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM saved_responses WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

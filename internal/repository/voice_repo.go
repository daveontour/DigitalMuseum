package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VoiceRepo accesses the custom_voices table.
type VoiceRepo struct {
	pool *pgxpool.Pool
}

// NewVoiceRepo creates a VoiceRepo.
func NewVoiceRepo(pool *pgxpool.Pool) *VoiceRepo {
	return &VoiceRepo{pool: pool}
}

const voiceCols = `id, key, name, description, instructions, creativity, created_at, updated_at`

func scanVoice(row interface{ Scan(...any) error }) (*model.CustomVoice, error) {
	var v model.CustomVoice
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Description,
		&v.Instructions, &v.Creativity, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// List returns all custom voices ordered by name.
func (r *VoiceRepo) List(ctx context.Context) ([]*model.CustomVoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+voiceCols+` FROM custom_voices ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("ListVoices: %w", err)
	}
	defer rows.Close()
	var out []*model.CustomVoice
	for rows.Next() {
		v, err := scanVoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetByID returns a single custom voice.
func (r *VoiceRepo) GetByID(ctx context.Context, id int64) (*model.CustomVoice, error) {
	v, err := scanVoice(r.pool.QueryRow(ctx,
		`SELECT `+voiceCols+` FROM custom_voices WHERE id = $1`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

// GetByKey returns a custom voice by its slug key.
func (r *VoiceRepo) GetByKey(ctx context.Context, key string) (*model.CustomVoice, error) {
	v, err := scanVoice(r.pool.QueryRow(ctx,
		`SELECT `+voiceCols+` FROM custom_voices WHERE key = $1`, key))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

// KeyExistsExcluding returns true if another row with key exists (excluding given ID).
func (r *VoiceRepo) KeyExistsExcluding(ctx context.Context, key string, excludeID int64) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM custom_voices WHERE key=$1 AND id!=$2`, key, excludeID,
	).Scan(&n)
	return n > 0, err
}

// Create inserts a new custom voice.
func (r *VoiceRepo) Create(ctx context.Context, key, name string, description *string, instructions string, creativity float64) (*model.CustomVoice, error) {
	v, err := scanVoice(r.pool.QueryRow(ctx,
		`INSERT INTO custom_voices (key, name, description, instructions, creativity)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+voiceCols,
		key, name, description, instructions, creativity,
	))
	if err != nil {
		return nil, fmt.Errorf("CreateVoice: %w", err)
	}
	return v, nil
}

// Update modifies a custom voice.
func (r *VoiceRepo) Update(ctx context.Context, id int64, key, name *string, description *string, instructions *string, creativity *float64) (*model.CustomVoice, error) {
	v, err := scanVoice(r.pool.QueryRow(ctx,
		`UPDATE custom_voices SET
		 key          = COALESCE($1, key),
		 name         = COALESCE($2, name),
		 description  = COALESCE($3, description),
		 instructions = COALESCE($4, instructions),
		 creativity   = COALESCE($5, creativity),
		 updated_at   = NOW()
		 WHERE id = $6
		 RETURNING `+voiceCols,
		key, name, description, instructions, creativity, id,
	))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("UpdateVoice %d: %w", id, err)
	}
	return v, nil
}

// Delete removes a custom voice.
func (r *VoiceRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM custom_voices WHERE id = $1`, id)
	return err
}

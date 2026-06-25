package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// VoiceRepo accesses the custom_voices table.
type VoiceRepo struct {
	pool *sql.DB
}

// NewVoiceRepo creates a VoiceRepo.
func NewVoiceRepo(pool *sql.DB) *VoiceRepo {
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
	uid := uidFromCtx(ctx)
	q := `SELECT ` + voiceCols + ` FROM custom_voices WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY name"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listVoices: %w", err)
	}
	defer func() { _ = rows.Close() }()
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
	uid := uidFromCtx(ctx)
	q := `SELECT ` + voiceCols + ` FROM custom_voices WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	v, err := scanVoice(r.pool.QueryRowContext(ctx, q, args...))
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
	uid := uidFromCtx(ctx)
	q := `SELECT ` + voiceCols + ` FROM custom_voices WHERE key = ?1`
	args := []any{key}
	q, args = addUIDFilter(q, args, uid)
	v, err := scanVoice(r.pool.QueryRowContext(ctx, q, args...))
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
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM custom_voices WHERE key=?1 AND id!=?2`
	args := []any{key, excludeID}
	q, args = addUIDFilter(q, args, uid)
	var n int
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n > 0, err
}

// Create inserts a new custom voice.
func (r *VoiceRepo) Create(ctx context.Context, key, name string, description *string, instructions string, creativity float64) (*model.CustomVoice, error) {
	uid := uidFromCtx(ctx)
	v, err := scanVoice(r.pool.QueryRowContext(ctx,
		`INSERT INTO custom_voices (key, name, description, instructions, creativity, user_id)
		 VALUES (?1,?2,?3,?4,?5,?6) RETURNING `+voiceCols,
		key, name, description, instructions, creativity, uidVal(uid),
	))
	if err != nil {
		return nil, fmt.Errorf("createVoice: %w", err)
	}
	return v, nil
}

// Update modifies a custom voice.
func (r *VoiceRepo) Update(ctx context.Context, id int64, key, name *string, description *string, instructions *string, creativity *float64) (*model.CustomVoice, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE custom_voices SET
	      key          = COALESCE(?1, key),
	      name         = COALESCE(?2, name),
	      description  = COALESCE(?3, description),
	      instructions = COALESCE(?4, instructions),
	      creativity   = COALESCE(?5, creativity),
	      updated_at   = CURRENT_TIMESTAMP
	      WHERE id = ?6`
	args := []any{key, name, description, instructions, creativity, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING ` + voiceCols
	v, err := scanVoice(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("updateVoice %d: %w", id, err)
	}
	return v, nil
}

// Delete removes a custom voice.
func (r *VoiceRepo) Delete(ctx context.Context, id int64) error {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM custom_voices WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

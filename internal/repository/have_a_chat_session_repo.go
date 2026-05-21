package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
)

// HaveAChatSessionRepo persists Have a Chat conversation snapshots.
type HaveAChatSessionRepo struct {
	pool *sql.DB
}

// NewHaveAChatSessionRepo creates a HaveAChatSessionRepo.
func NewHaveAChatSessionRepo(pool *sql.DB) *HaveAChatSessionRepo {
	return &HaveAChatSessionRepo{pool: pool}
}

// Create inserts a completed session row and returns its id.
func (r *HaveAChatSessionRepo) Create(ctx context.Context, in model.HaveAChatSessionSave) (int64, error) {
	uid := uidFromCtx(ctx)
	histJSON, err := json.Marshal(in.History)
	if err != nil {
		return 0, fmt.Errorf("HaveAChatSessionRepo.Create marshal history: %w", err)
	}
	stopped := time.Now().UTC()
	var id int64
	err = r.pool.QueryRowContext(ctx,
		`INSERT INTO have_a_chat_sessions (
			user_id, stopped_at, topic, voice_a, voice_b, provider_a, provider_b,
			banter_mode, temperature, allow_explicit, turn_count, history_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id`,
		uidVal(uid),
		stopped.Format(time.RFC3339),
		in.Topic,
		in.VoiceA, in.VoiceB, in.ProviderA, in.ProviderB,
		in.BanterMode,
		in.Temperature,
		in.AllowExplicit,
		in.TurnCount,
		string(histJSON),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("HaveAChatSessionRepo.Create: %w", err)
	}
	return id, nil
}

// ListRecent returns the most recent sessions for the current user (newest first).
func (r *HaveAChatSessionRepo) ListRecent(ctx context.Context, limit int) ([]model.HaveAChatSessionListItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	uid := uidFromCtx(ctx)
	q := `SELECT id, created_at, stopped_at, topic, turn_count
	      FROM have_a_chat_sessions WHERE 1=1`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY COALESCE(stopped_at, created_at) DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("HaveAChatSessionRepo.ListRecent: %w", err)
	}
	defer rows.Close()
	var out []model.HaveAChatSessionListItem
	for rows.Next() {
		var it model.HaveAChatSessionListItem
		var createdAt, stoppedAt sql.NullString
		if err := rows.Scan(&it.ID, &createdAt, &stoppedAt, &it.Topic, &it.TurnCount); err != nil {
			return nil, err
		}
		it.CreatedAt = createdAt.String
		if stoppedAt.Valid {
			it.StoppedAt = stoppedAt.String
		}
		out = append(out, it)
	}
	if out == nil {
		out = []model.HaveAChatSessionListItem{}
	}
	return out, rows.Err()
}

// GetByID returns a full session for the current user, or nil if not found.
func (r *HaveAChatSessionRepo) GetByID(ctx context.Context, id int64) (*model.HaveAChatSessionDetail, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, created_at, stopped_at, topic, voice_a, voice_b, provider_a, provider_b,
	             banter_mode, temperature, allow_explicit, turn_count, history_json
	      FROM have_a_chat_sessions WHERE id = ?`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	var d model.HaveAChatSessionDetail
	var createdAt, stoppedAt sql.NullString
	var histJSON string
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(
		&d.ID, &createdAt, &stoppedAt, &d.Topic,
		&d.VoiceA, &d.VoiceB, &d.ProviderA, &d.ProviderB,
		&d.BanterMode, &d.Temperature, &d.AllowExplicit, &d.TurnCount, &histJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("HaveAChatSessionRepo.GetByID: %w", err)
	}
	d.CreatedAt = createdAt.String
	if stoppedAt.Valid {
		d.StoppedAt = stoppedAt.String
	}
	if err := json.Unmarshal([]byte(histJSON), &d.History); err != nil {
		return nil, fmt.Errorf("HaveAChatSessionRepo.GetByID decode history: %w", err)
	}
	if d.History == nil {
		d.History = []model.HaveAChatTurn{}
	}
	return &d, nil
}

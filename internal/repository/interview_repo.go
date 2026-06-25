package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
)

// InterviewRepo accesses the interviews and interview_turns tables.
type InterviewRepo struct {
	pool *sql.DB
}

// NewInterviewRepo creates an InterviewRepo.
func NewInterviewRepo(pool *sql.DB) *InterviewRepo {
	return &InterviewRepo{pool: pool}
}

// CreateInterview inserts a new interviews row.
func (r *InterviewRepo) CreateInterview(ctx context.Context, title, style, purpose, purposeDetail, provider string) (*model.Interview, error) {
	uid := uidFromCtx(ctx)
	var iv model.Interview
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO interviews (title, style, purpose, purpose_detail, provider, user_id)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6)
		 RETURNING id, title, style, purpose, purpose_detail, state, provider, created_at, updated_at, last_turn_at`,
		title, style, purpose, purposeDetail, provider, uidVal(uid),
	).Scan(&iv.ID, &iv.Title, &iv.Style, &iv.Purpose, &iv.PurposeDetail,
		&iv.State, &iv.Provider, &iv.CreatedAt, &iv.UpdatedAt, &iv.LastTurnAt)
	if err != nil {
		return nil, fmt.Errorf("createInterview: %w", err)
	}
	return &iv, nil
}

// GetInterview returns a single interview by ID, or nil if not found.
func (r *InterviewRepo) GetInterview(ctx context.Context, id int64) (*model.Interview, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT i.id, i.title, i.style, i.purpose, i.purpose_detail, i.state, i.provider,
	             i.writeup, i.created_at, i.updated_at, i.last_turn_at,
	             COALESCE((SELECT COUNT(*) FROM interview_turns WHERE interview_id = i.id), 0)
	      FROM interviews i WHERE i.id = ?1`
	args := []any{id}
	q, args = addUIDFilterQualified(q, args, uid, "i")
	var iv model.Interview
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(
		&iv.ID, &iv.Title, &iv.Style, &iv.Purpose, &iv.PurposeDetail,
		&iv.State, &iv.Provider, &iv.Writeup, &iv.CreatedAt, &iv.UpdatedAt, &iv.LastTurnAt,
		&iv.TurnCount,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("getInterview %d: %w", id, err)
	}
	iv.SetHasWriteupFromWriteup()
	return &iv, nil
}

// ListInterviews returns all interviews for the current user, ordered by most recent.
// Writeup text is not loaded (use GET /interview/{id} for full text); HasWriteup indicates storage.
func (r *InterviewRepo) ListInterviews(ctx context.Context, stateFilter string) ([]*model.Interview, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT i.id, i.title, i.style, i.purpose, i.purpose_detail, i.state, i.provider,
	             (i.writeup IS NOT NULL AND LENGTH(TRIM(COALESCE(i.writeup, ''))) > 0),
	             i.created_at, i.updated_at, i.last_turn_at,
	             COALESCE((SELECT COUNT(*) FROM interview_turns WHERE interview_id = i.id), 0)
	      FROM interviews i WHERE TRUE`
	args := []any{}
	q, args = addUIDFilterQualified(q, args, uid, "i")
	if stateFilter != "" {
		args = append(args, stateFilter)
		q += fmt.Sprintf(" AND i.state = ?%d", len(args))
	}
	q += " ORDER BY COALESCE(i.last_turn_at, i.created_at) DESC"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listInterviews: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.Interview
	for rows.Next() {
		var iv model.Interview
		if err := rows.Scan(&iv.ID, &iv.Title, &iv.Style, &iv.Purpose, &iv.PurposeDetail,
			&iv.State, &iv.Provider, &iv.HasWriteup, &iv.CreatedAt, &iv.UpdatedAt, &iv.LastTurnAt,
			&iv.TurnCount); err != nil {
			return nil, err
		}
		out = append(out, &iv)
	}
	return out, rows.Err()
}

// SaveWriteup stores the generated writeup text and marks the interview as finished.
func (r *InterviewRepo) SaveWriteup(ctx context.Context, id int64, writeup string) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE interviews SET writeup = ?1, state = 'finished', updated_at = CURRENT_TIMESTAMP WHERE id = ?2`
	args := []any{writeup, id}
	q, args = addUIDFilter(q, args, uid)
	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("saveWriteup: interview %d not found or not accessible", id)
	}
	return nil
}

// UpdateInterviewState sets the state and updated_at.
func (r *InterviewRepo) UpdateInterviewState(ctx context.Context, id int64, state string) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE interviews SET state = ?1, updated_at = CURRENT_TIMESTAMP WHERE id = ?2`
	args := []any{state, id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// SaveTurn inserts a new interview_turns row (question only; answer is NULL)
// and updates last_turn_at on the parent interview.
func (r *InterviewRepo) SaveTurn(ctx context.Context, interviewID int64, question string) (*model.InterviewTurn, error) {
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("saveTurn begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var t model.InterviewTurn
	err = tx.QueryRowContext(ctx,
		`INSERT INTO interview_turns (interview_id, turn_number, question, user_id)
		 VALUES (?1,
		   COALESCE((SELECT MAX(turn_number) FROM interview_turns WHERE interview_id = ?1), 0) + 1,
		   ?2, ?3)
		 RETURNING id, interview_id, question, answer, turn_number, created_at`,
		interviewID, question, uidVal(uid),
	).Scan(&t.ID, &t.InterviewID, &t.Question, &t.Answer, &t.TurnNumber, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("saveTurn insert: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE interviews SET last_turn_at = ?1, updated_at = CURRENT_TIMESTAMP WHERE id = ?2`,
		time.Now(), interviewID)
	if err != nil {
		return nil, fmt.Errorf("saveTurn update interview: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("saveTurn commit: %w", err)
	}
	return &t, nil
}

// SaveAnswer updates the answer column on the latest unanswered turn.
func (r *InterviewRepo) SaveAnswer(ctx context.Context, interviewID int64, answer string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE interview_turns SET answer = ?1
		 WHERE interview_id = ?2 AND answer IS NULL
		 ORDER BY turn_number DESC LIMIT 1`,
		answer, interviewID)
	if err != nil {
		// Fallback: some Postgres versions don't support ORDER BY + LIMIT on UPDATE.
		_, err = r.pool.ExecContext(ctx,
			`UPDATE interview_turns SET answer = ?1
			 WHERE id = (
			   SELECT id FROM interview_turns
			   WHERE interview_id = ?2 AND answer IS NULL
			   ORDER BY turn_number DESC LIMIT 1
			 )`,
			answer, interviewID)
	}
	return err
}

// GetTurns returns all turns for an interview in chronological order.
func (r *InterviewRepo) GetTurns(ctx context.Context, interviewID int64) ([]*model.InterviewTurn, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, interview_id, question, answer, turn_number, created_at
		 FROM interview_turns WHERE interview_id = ?1
		 ORDER BY turn_number ASC`,
		interviewID)
	if err != nil {
		return nil, fmt.Errorf("getTurns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.InterviewTurn
	for rows.Next() {
		var t model.InterviewTurn
		if err := rows.Scan(&t.ID, &t.InterviewID, &t.Question, &t.Answer, &t.TurnNumber, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// GetLastTurn returns the most recent turn for an interview, or nil.
func (r *InterviewRepo) GetLastTurn(ctx context.Context, interviewID int64) (*model.InterviewTurn, error) {
	var t model.InterviewTurn
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, interview_id, question, answer, turn_number, created_at
		 FROM interview_turns WHERE interview_id = ?1
		 ORDER BY turn_number DESC LIMIT 1`,
		interviewID,
	).Scan(&t.ID, &t.InterviewID, &t.Question, &t.Answer, &t.TurnNumber, &t.CreatedAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("getLastTurn: %w", err)
	}
	return &t, nil
}

// DeleteInterview removes an interview and its turns (cascade).
func (r *InterviewRepo) DeleteInterview(ctx context.Context, id int64) error {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM interviews WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

package repository

import (
	"context"
	"database/sql"
	"fmt"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// PamBotSession holds state for a single Pam Bot companion session.
type PamBotSession struct {
	ID                  int
	UserID              *int64
	StartedAt           sqlutil.DBTime
	LastInteractionAt   sqlutil.DBTime
	InteractionCount    int
	LatestSummary       *string
	LatestAnalysis      *string
	LatestSummaryAt     sqlutil.NullDBTime
	LastFacebookPostID  *int64
	LastFacebookAlbumID *int64
}

// PamBotSubject tracks a topic that has been discussed.
type PamBotSubject struct {
	SubjectTag      string
	SubjectCategory string
	LastDiscussedAt sqlutil.DBTime
	DiscussCount    int
}

// PamBotTurn holds one exchange in a session.
type PamBotTurn struct {
	TurnNumber      int
	SubjectTag      string
	SubjectCategory string
	BotMessage      string
	UserAction      string
}

// PamBotRepo provides all DB access for the Pam Bot feature.
type PamBotRepo struct {
	pool *sql.DB
}

// NewPamBotRepo creates a PamBotRepo.
func NewPamBotRepo(pool *sql.DB) *PamBotRepo {
	return &PamBotRepo{pool: pool}
}

// GetOrCreateSession returns the most recent session for the user (creating one if none exists).
func (r *PamBotRepo) GetOrCreateSession(ctx context.Context) (*PamBotSession, error) {
	uid := uidFromCtx(ctx)
	uidArg := uidVal(uid)

	// Try to get the most recent session
	q := `SELECT id, user_id, started_at, last_interaction_at, interaction_count,
	             latest_summary, latest_analysis, latest_summary_at,
	             last_facebook_post_id, last_facebook_album_id
	      FROM pam_bot_sessions
	      WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY started_at DESC LIMIT 1"

	s := &PamBotSession{}
	var lastFBPost, lastFBAlbum sql.NullInt64
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(
		&s.ID, &s.UserID, &s.StartedAt, &s.LastInteractionAt, &s.InteractionCount,
		&s.LatestSummary, &s.LatestAnalysis, &s.LatestSummaryAt,
		&lastFBPost, &lastFBAlbum,
	)
	if err == nil {
		if lastFBPost.Valid {
			v := lastFBPost.Int64
			s.LastFacebookPostID = &v
		}
		if lastFBAlbum.Valid {
			v := lastFBAlbum.Int64
			s.LastFacebookAlbumID = &v
		}
		return s, nil
	}
	if !isNoRows(err) {
		return nil, fmt.Errorf("GetOrCreateSession query: %w", err)
	}

	// Create a new session
	err = r.pool.QueryRowContext(ctx,
		`INSERT INTO pam_bot_sessions (user_id) VALUES (?1)
		 RETURNING id, user_id, started_at, last_interaction_at, interaction_count,
		           latest_summary, latest_analysis, latest_summary_at,
		           last_facebook_post_id, last_facebook_album_id`,
		uidArg,
	).Scan(
		&s.ID, &s.UserID, &s.StartedAt, &s.LastInteractionAt, &s.InteractionCount,
		&s.LatestSummary, &s.LatestAnalysis, &s.LatestSummaryAt,
		&lastFBPost, &lastFBAlbum,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateSession insert: %w", err)
	}
	if lastFBPost.Valid {
		v := lastFBPost.Int64
		s.LastFacebookPostID = &v
	}
	if lastFBAlbum.Valid {
		v := lastFBAlbum.Int64
		s.LastFacebookAlbumID = &v
	}
	return s, nil
}

// GetRecentSubjects returns the most recently discussed subjects, most recent first.
func (r *PamBotRepo) GetRecentSubjects(ctx context.Context, limit int) ([]PamBotSubject, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT subject_tag, COALESCE(subject_category,''), last_discussed_at, discuss_count
	      FROM pam_bot_subjects
	      WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY last_discussed_at DESC LIMIT ?%d", len(args))

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetRecentSubjects: %w", err)
	}
	defer rows.Close()

	var subjects []PamBotSubject
	for rows.Next() {
		var s PamBotSubject
		if err := rows.Scan(&s.SubjectTag, &s.SubjectCategory, &s.LastDiscussedAt, &s.DiscussCount); err != nil {
			return nil, fmt.Errorf("GetRecentSubjects scan: %w", err)
		}
		subjects = append(subjects, s)
	}
	return subjects, rows.Err()
}

// GetRecentTurns returns the last N turns of a session as ConvTurn history for the LLM.
func (r *PamBotRepo) GetRecentTurns(ctx context.Context, sessionID, limit int) ([]appai.ConvTurn, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT user_action, bot_message FROM pam_bot_turns
		 WHERE session_id = ?1
		 ORDER BY turn_number DESC LIMIT ?2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("GetRecentTurns: %w", err)
	}
	defer rows.Close()

	var turns []appai.ConvTurn
	for rows.Next() {
		var action, message string
		if err := rows.Scan(&action, &message); err != nil {
			return nil, fmt.Errorf("GetRecentTurns scan: %w", err)
		}
		turns = append(turns, appai.ConvTurn{UserInput: action, ResponseText: message})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse so oldest is first
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns, nil
}

// GetRecentTurnsRaw returns turns in chronological order for summary generation.
func (r *PamBotRepo) GetRecentTurnsRaw(ctx context.Context, sessionID, limit int) ([]PamBotTurn, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT turn_number, COALESCE(subject_tag,''), COALESCE(subject_category,''),
		        bot_message, COALESCE(user_action,'')
		 FROM pam_bot_turns
		 WHERE session_id = ?1
		 ORDER BY turn_number DESC LIMIT ?2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("GetRecentTurnsRaw: %w", err)
	}
	defer rows.Close()

	var turns []PamBotTurn
	for rows.Next() {
		var t PamBotTurn
		if err := rows.Scan(&t.TurnNumber, &t.SubjectTag, &t.SubjectCategory, &t.BotMessage, &t.UserAction); err != nil {
			return nil, fmt.Errorf("GetRecentTurnsRaw scan: %w", err)
		}
		turns = append(turns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns, nil
}

// SaveTurn records a single Pam Bot exchange.
func (r *PamBotRepo) SaveTurn(ctx context.Context, sessionID, turnNumber int, subjectTag, subjectCategory, botMessage, userAction string) error {
	uid := uidVal(uidFromCtx(ctx))
	var tagPtr, catPtr *string
	if subjectTag != "" {
		tagPtr = &subjectTag
	}
	if subjectCategory != "" {
		catPtr = &subjectCategory
	}
	_, err := r.pool.ExecContext(ctx,
		`INSERT INTO pam_bot_turns (user_id, session_id, turn_number, subject_tag, subject_category, bot_message, user_action)
		 VALUES (?1,?2,?3,?4,?5,?6,?7)`,
		uid, sessionID, turnNumber, tagPtr, catPtr, botMessage, userAction,
	)
	if err != nil {
		return fmt.Errorf("SaveTurn: %w", err)
	}
	return nil
}

// UpsertSubject creates or updates the subject tracking record.
func (r *PamBotRepo) UpsertSubject(ctx context.Context, subjectTag, subjectCategory string) error {
	uid := uidVal(uidFromCtx(ctx))
	var catPtr *string
	if subjectCategory != "" {
		catPtr = &subjectCategory
	}
	_, err := r.pool.ExecContext(ctx,
		`INSERT INTO pam_bot_subjects (user_id, subject_tag, subject_category, last_discussed_at, discuss_count)
		 VALUES (?1, ?2, ?3, CURRENT_TIMESTAMP, 1)
		 ON CONFLICT (user_id, subject_tag)
		 DO UPDATE SET last_discussed_at = CURRENT_TIMESTAMP,
		               discuss_count = pam_bot_subjects.discuss_count + 1,
		               subject_category = EXCLUDED.subject_category`,
		uid, subjectTag, catPtr,
	)
	if err != nil {
		return fmt.Errorf("UpsertSubject: %w", err)
	}
	return nil
}

// IncrementInteractionCount updates the session count and timestamp.
func (r *PamBotRepo) IncrementInteractionCount(ctx context.Context, sessionID int) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE pam_bot_sessions
		 SET interaction_count = interaction_count + 1, last_interaction_at = CURRENT_TIMESTAMP
		 WHERE id = ?1`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("IncrementInteractionCount: %w", err)
	}
	return nil
}

// UpdateSessionSummary saves the LLM-generated summary and cognitive analysis.
func (r *PamBotRepo) UpdateSessionSummary(ctx context.Context, sessionID int, summary, analysis string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE pam_bot_sessions
		 SET latest_summary = ?1, latest_analysis = ?2, latest_summary_at = CURRENT_TIMESTAMP
		 WHERE id = ?3`,
		summary, analysis, sessionID,
	)
	if err != nil {
		return fmt.Errorf("UpdateSessionSummary: %w", err)
	}
	return nil
}

// UpdateSessionFacebookContext stores facebook_post_id / facebook_album_id from the last assistant
// <json> block (0 clears that column to NULL).
func (r *PamBotRepo) UpdateSessionFacebookContext(ctx context.Context, sessionID int, postID, albumID int64) error {
	var postPtr, albumPtr *int64
	if postID != 0 {
		postPtr = &postID
	}
	if albumID != 0 {
		albumPtr = &albumID
	}
	_, err := r.pool.ExecContext(ctx,
		`UPDATE pam_bot_sessions SET last_facebook_post_id = ?2, last_facebook_album_id = ?3 WHERE id = ?1`,
		sessionID, postPtr, albumPtr,
	)
	if err != nil {
		return fmt.Errorf("UpdateSessionFacebookContext: %w", err)
	}
	return nil
}

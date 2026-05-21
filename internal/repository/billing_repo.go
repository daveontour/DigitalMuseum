package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// BillingRepo records LLM usage in the billing database (separate from main app DB).
type BillingRepo struct {
	pool *sql.DB
}

// NewBillingRepo creates a BillingRepo. pool may be nil to disable recording.
func NewBillingRepo(pool *sql.DB) *BillingRepo {
	return &BillingRepo{pool: pool}
}

// LLMUsageEvent is one row in llm_usage_events.
type LLMUsageEvent struct {
	ID               int64          `json:"id"`
	CreatedAt        sqlutil.DBTime `json:"created_at"`
	Provider         string         `json:"provider"`
	UserID           *int64         `json:"user_id,omitempty"`
	IsVisitor        bool           `json:"is_visitor"`
	InputTokens      int            `json:"input_tokens"`
	OutputTokens     int            `json:"output_tokens"`
	ModelName        *string        `json:"model_name,omitempty"`
	UserEmail        *string        `json:"user_email,omitempty"`
	UserFirstName    *string        `json:"user_first_name,omitempty"`
	UserFamilyName   *string        `json:"user_family_name,omitempty"`
	UsedServerLLMKey *bool          `json:"used_server_llm_key,omitempty"`
	Succeeded        bool           `json:"succeeded"`
	ErrorMessage     *string        `json:"error_message,omitempty"`
}

// InsertLLMUsage appends a usage event. No-op if pool is nil.
// userEmail, userFirstName, userFamilyName are optional snapshots from the main users table at insert time.
// usedServerLLMKey is nil when unknown (legacy rows); true means env/server key, false means user or visitor override.
// succeeded is false when the LLM API call failed; errorMessage is set only for failures (truncated by caller if needed).
func (r *BillingRepo) InsertLLMUsage(ctx context.Context, provider string, userID *int64, isVisitor bool, inputTokens, outputTokens int, modelName *string, userEmail, userFirstName, userFamilyName *string, usedServerLLMKey *bool, succeeded bool, errorMessage *string) error {
	if r == nil || r.pool == nil {
		return nil
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	_, err := r.pool.ExecContext(ctx, `
		INSERT INTO llm_usage_events (provider, user_id, is_visitor, input_tokens, output_tokens, model_name, user_email, user_first_name, user_family_name, used_server_llm_key, succeeded, error_message)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)`,
		provider, userID, isVisitor, inputTokens, outputTokens, modelName, userEmail, userFirstName, userFamilyName, usedServerLLMKey, succeeded, errorMessage,
	)
	return err
}

// LLMUsageSummary aggregates totals for a user in a time window.
type LLMUsageSummary struct {
	TotalInputTokens  int64
	TotalOutputTokens int64
	EventCount        int64
}

// ProviderBreakdown is per-provider totals.
type ProviderBreakdown struct {
	Provider     string `json:"provider"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	EventCount   int64  `json:"event_count"`
}

// VisitorBreakdown splits owner vs visitor sessions.
type VisitorBreakdown struct {
	IsVisitor    bool  `json:"is_visitor"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	EventCount   int64 `json:"event_count"`
}

// SummaryByUser returns aggregate totals and optional breakdowns.
func (r *BillingRepo) SummaryByUser(ctx context.Context, userID int64, from, to *time.Time) (sum LLMUsageSummary, byProvider []ProviderBreakdown, byVisitor []VisitorBreakdown, err error) {
	if r == nil || r.pool == nil {
		return sum, nil, nil, errors.New("billing: not configured")
	}
	q := `SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COUNT(*) FROM llm_usage_events WHERE user_id = ?1`
	args := []any{userID}
	n := 2
	if from != nil {
		q += fmt.Sprintf(" AND created_at >= ?%d", n)
		args = append(args, *from)
		n++
	}
	if to != nil {
		q += fmt.Sprintf(" AND created_at < ?%d", n)
		args = append(args, *to)
	}
	err = r.pool.QueryRowContext(ctx, q, args...).Scan(&sum.TotalInputTokens, &sum.TotalOutputTokens, &sum.EventCount)
	if err != nil {
		return sum, nil, nil, err
	}

	q2 := `SELECT provider, COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COUNT(*) FROM llm_usage_events WHERE user_id = ?1`
	args2 := []any{userID}
	n2 := 2
	if from != nil {
		q2 += fmt.Sprintf(" AND created_at >= ?%d", n2)
		args2 = append(args2, *from)
		n2++
	}
	if to != nil {
		q2 += fmt.Sprintf(" AND created_at < ?%d", n2)
		args2 = append(args2, *to)
	}
	q2 += ` GROUP BY provider ORDER BY provider`
	rows, err := r.pool.QueryContext(ctx, q2, args2...)
	if err != nil {
		return sum, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p ProviderBreakdown
		if err := rows.Scan(&p.Provider, &p.InputTokens, &p.OutputTokens, &p.EventCount); err != nil {
			return sum, nil, nil, err
		}
		byProvider = append(byProvider, p)
	}
	if err := rows.Err(); err != nil {
		return sum, nil, nil, err
	}

	q3 := `SELECT is_visitor, COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COUNT(*) FROM llm_usage_events WHERE user_id = ?1`
	args3 := []any{userID}
	n3 := 2
	if from != nil {
		q3 += fmt.Sprintf(" AND created_at >= ?%d", n3)
		args3 = append(args3, *from)
		n3++
	}
	if to != nil {
		q3 += fmt.Sprintf(" AND created_at < ?%d", n3)
		args3 = append(args3, *to)
	}
	q3 += ` GROUP BY is_visitor ORDER BY is_visitor`
	rows2, err := r.pool.QueryContext(ctx, q3, args3...)
	if err != nil {
		return sum, byProvider, nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var v VisitorBreakdown
		if err := rows2.Scan(&v.IsVisitor, &v.InputTokens, &v.OutputTokens, &v.EventCount); err != nil {
			return sum, byProvider, nil, err
		}
		byVisitor = append(byVisitor, v)
	}
	return sum, byProvider, byVisitor, rows2.Err()
}

// ListEventsByUser returns paginated events for the detail table.
func (r *BillingRepo) ListEventsByUser(ctx context.Context, userID int64, from, to *time.Time, limit, offset int) ([]LLMUsageEvent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("billing: not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := `SELECT id, created_at, provider, user_id, is_visitor, input_tokens, output_tokens, model_name,
		user_email, user_first_name, user_family_name, used_server_llm_key,
		COALESCE(succeeded, TRUE), error_message
		FROM llm_usage_events WHERE user_id = ?1`
	args := []any{userID}
	n := 2
	if from != nil {
		q += fmt.Sprintf(" AND created_at >= ?%d", n)
		args = append(args, *from)
		n++
	}
	if to != nil {
		q += fmt.Sprintf(" AND created_at < ?%d", n)
		args = append(args, *to)
		n++
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT ?%d OFFSET ?%d`, n, n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LLMUsageEvent
	for rows.Next() {
		var e LLMUsageEvent
		var model, email, firstName, familyName, errMsg sql.NullString
		var usedSrv sql.NullBool
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Provider, &e.UserID, &e.IsVisitor, &e.InputTokens, &e.OutputTokens, &model,
			&email, &firstName, &familyName, &usedSrv, &e.Succeeded, &errMsg); err != nil {
			return nil, err
		}
		if model.Valid {
			s := model.String
			e.ModelName = &s
		}
		if email.Valid {
			s := email.String
			e.UserEmail = &s
		}
		if firstName.Valid {
			s := firstName.String
			e.UserFirstName = &s
		}
		if familyName.Valid {
			s := familyName.String
			e.UserFamilyName = &s
		}
		if usedSrv.Valid {
			b := usedSrv.Bool
			e.UsedServerLLMKey = &b
		}
		if errMsg.Valid {
			s := errMsg.String
			e.ErrorMessage = &s
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListFailedEvents returns paginated LLM usage rows where succeeded is false.
// If userID is nil, events for all users are returned (newest first).
func (r *BillingRepo) ListFailedEvents(ctx context.Context, userID *int64, from, to *time.Time, limit, offset int) ([]LLMUsageEvent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("billing: not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := `SELECT id, created_at, provider, user_id, is_visitor, input_tokens, output_tokens, model_name,
		user_email, user_first_name, user_family_name, used_server_llm_key,
		COALESCE(succeeded, TRUE), error_message
		FROM llm_usage_events WHERE succeeded = false`
	args := []any{}
	n := 1
	if userID != nil {
		q += fmt.Sprintf(" AND user_id = ?%d", n)
		args = append(args, *userID)
		n++
	}
	if from != nil {
		q += fmt.Sprintf(" AND created_at >= ?%d", n)
		args = append(args, *from)
		n++
	}
	if to != nil {
		q += fmt.Sprintf(" AND created_at < ?%d", n)
		args = append(args, *to)
		n++
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT ?%d OFFSET ?%d`, n, n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LLMUsageEvent
	for rows.Next() {
		var e LLMUsageEvent
		var model, email, firstName, familyName, errMsg sql.NullString
		var usedSrv sql.NullBool
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Provider, &e.UserID, &e.IsVisitor, &e.InputTokens, &e.OutputTokens, &model,
			&email, &firstName, &familyName, &usedSrv, &e.Succeeded, &errMsg); err != nil {
			return nil, err
		}
		if model.Valid {
			s := model.String
			e.ModelName = &s
		}
		if email.Valid {
			s := email.String
			e.UserEmail = &s
		}
		if firstName.Valid {
			s := firstName.String
			e.UserFirstName = &s
		}
		if familyName.Valid {
			s := familyName.String
			e.UserFamilyName = &s
		}
		if usedSrv.Valid {
			b := usedSrv.Bool
			e.UsedServerLLMKey = &b
		}
		if errMsg.Valid {
			s := errMsg.String
			e.ErrorMessage = &s
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DefaultListEventsAllMax is the default row cap for ListEventsByUserAll (PDF export).
const DefaultListEventsAllMax = 50000

// ListEventsByUserAll returns events in chronological order (oldest first), up to maxRows inclusive.
// If more than maxRows rows exist in the time range, Truncated is true and only the first maxRows rows are returned.
// When maxRows <= 0, defaultListEventsAllMax is used.
func (r *BillingRepo) ListEventsByUserAll(ctx context.Context, userID int64, from, to *time.Time, maxRows int) (events []LLMUsageEvent, truncated bool, err error) {
	if r == nil || r.pool == nil {
		return nil, false, errors.New("billing: not configured")
	}
	if maxRows <= 0 {
		maxRows = DefaultListEventsAllMax
	}
	limit := maxRows + 1
	q := `SELECT id, created_at, provider, user_id, is_visitor, input_tokens, output_tokens, model_name,
		user_email, user_first_name, user_family_name, used_server_llm_key,
		COALESCE(succeeded, TRUE), error_message
		FROM llm_usage_events WHERE user_id = ?1`
	args := []any{userID}
	n := 2
	if from != nil {
		q += fmt.Sprintf(" AND created_at >= ?%d", n)
		args = append(args, *from)
		n++
	}
	if to != nil {
		q += fmt.Sprintf(" AND created_at < ?%d", n)
		args = append(args, *to)
		n++
	}
	q += fmt.Sprintf(` ORDER BY created_at ASC LIMIT ?%d`, n)
	args = append(args, limit)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var e LLMUsageEvent
		var model, email, firstName, familyName, errMsg sql.NullString
		var usedSrv sql.NullBool
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Provider, &e.UserID, &e.IsVisitor, &e.InputTokens, &e.OutputTokens, &model,
			&email, &firstName, &familyName, &usedSrv, &e.Succeeded, &errMsg); err != nil {
			return nil, false, err
		}
		if model.Valid {
			s := model.String
			e.ModelName = &s
		}
		if email.Valid {
			s := email.String
			e.UserEmail = &s
		}
		if firstName.Valid {
			s := firstName.String
			e.UserFirstName = &s
		}
		if familyName.Valid {
			s := familyName.String
			e.UserFamilyName = &s
		}
		if usedSrv.Valid {
			b := usedSrv.Bool
			e.UsedServerLLMKey = &b
		}
		if errMsg.Valid {
			s := errMsg.String
			e.ErrorMessage = &s
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(events) > maxRows {
		truncated = true
		events = events[:maxRows]
	}
	return events, truncated, nil
}

// TimeseriesBucket is one 5-minute aggregate for charting.
type TimeseriesBucket struct {
	BucketStart  time.Time
	InputTokens  int64
	OutputTokens int64
}

// TimeseriesByUser5Min aggregates token sums into 300-second buckets (UTC epoch alignment).
func (r *BillingRepo) TimeseriesByUser5Min(ctx context.Context, userID int64, from, to *time.Time) ([]TimeseriesBucket, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("billing: not configured")
	}
	var q string
	if sqlutil.IsSQLite(ctx, r.pool) {
		q = `
		SELECT datetime((CAST(strftime('%s', created_at) AS INTEGER) / 300) * 300, 'unixepoch') AS bucket_start,
			CAST(COALESCE(SUM(input_tokens), 0) AS INTEGER),
			CAST(COALESCE(SUM(output_tokens), 0) AS INTEGER)
		FROM llm_usage_events
		WHERE user_id = ?1`
	} else {
		q = `
		SELECT to_timestamp(floor(extract(epoch FROM created_at) / 300) * 300) AS bucket_start,
			COALESCE(SUM(input_tokens), 0)::bigint,
			COALESCE(SUM(output_tokens), 0)::bigint
		FROM llm_usage_events
		WHERE user_id = ?1`
	}
	args := []any{userID}
	n := 2
	if from != nil {
		q += fmt.Sprintf(" AND created_at >= ?%d", n)
		args = append(args, *from)
		n++
	}
	if to != nil {
		q += fmt.Sprintf(" AND created_at < ?%d", n)
		args = append(args, *to)
	}
	q += ` GROUP BY 1 ORDER BY 1`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimeseriesBucket
	for rows.Next() {
		var b TimeseriesBucket
		var bucketStart sqlutil.DBTime
		if err := rows.Scan(&bucketStart, &b.InputTokens, &b.OutputTokens); err != nil {
			return nil, err
		}
		b.BucketStart = bucketStart.Time
		out = append(out, b)
	}
	return out, rows.Err()
}

// PgxPool returns the underlying pool for health checks (may be nil).
func (r *BillingRepo) PgxPool() *sql.DB {
	if r == nil {
		return nil
	}
	return r.pool
}

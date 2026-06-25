package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// BackgroundJobRepo accesses the background_jobs table — per-user scheduling
// state for the named maintenance jobs surfaced in the Configuration UI.
type BackgroundJobRepo struct {
	pool *sql.DB
}

// NewBackgroundJobRepo creates a BackgroundJobRepo.
func NewBackgroundJobRepo(pool *sql.DB) *BackgroundJobRepo {
	return &BackgroundJobRepo{pool: pool}
}

const backgroundJobCols = `id, job_name, auto_start, restart_on_complete,
	interval_seconds, last_run_at, last_run_result, last_run_message,
	next_due_at, created_at, updated_at, user_id`

func scanBackgroundJob(row interface{ Scan(...any) error }) (*model.BackgroundJob, error) {
	var j model.BackgroundJob
	err := row.Scan(
		&j.ID, &j.JobName, &j.AutoStart, &j.RestartOnComplete,
		&j.IntervalSeconds, &j.LastRunAt, &j.LastRunResult, &j.LastRunMessage,
		&j.NextDueAt, &j.CreatedAt, &j.UpdatedAt, &j.UserID,
	)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// List returns all background_jobs rows for the calling user (newest job first).
func (r *BackgroundJobRepo) List(ctx context.Context) ([]*model.BackgroundJob, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + backgroundJobCols + ` FROM background_jobs WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY job_name ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listBackgroundJobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.BackgroundJob
	for rows.Next() {
		j, err := scanBackgroundJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// GetByName returns the row for (user, jobName), or nil when absent.
func (r *BackgroundJobRepo) GetByName(ctx context.Context, jobName string) (*model.BackgroundJob, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + backgroundJobCols + ` FROM background_jobs WHERE job_name = ?1`
	args := []any{jobName}
	q, args = addUIDFilter(q, args, uid)
	j, err := scanBackgroundJob(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return j, nil
}

// Upsert sets auto_start / restart_on_complete / interval_seconds for (user, jobName).
// Other tracking fields (last_run_*, next_due_at) are left untouched on update.
func (r *BackgroundJobRepo) Upsert(ctx context.Context, jobName string, autoStart, restartOnComplete bool, intervalSeconds int64) (*model.BackgroundJob, error) {
	uid := uidFromCtx(ctx)
	conflictClause := `ON CONFLICT (user_id, job_name) DO UPDATE SET`
	if sqlutil.IsSQLite(ctx, r.pool) {
		conflictClause = `ON CONFLICT (user_id, job_name) WHERE user_id IS NOT NULL DO UPDATE SET`
		if uid == 0 {
			conflictClause = `ON CONFLICT (job_name) WHERE user_id IS NULL DO UPDATE SET`
		}
	}
	j, err := scanBackgroundJob(r.pool.QueryRowContext(ctx,
		`INSERT INTO background_jobs (job_name, auto_start, restart_on_complete, interval_seconds, user_id)
		 VALUES (?1, ?2, ?3, ?4, ?5)
		 `+conflictClause+`
		   auto_start          = EXCLUDED.auto_start,
		   restart_on_complete = EXCLUDED.restart_on_complete,
		   interval_seconds    = EXCLUDED.interval_seconds,
		   updated_at          = CURRENT_TIMESTAMP
		 RETURNING `+backgroundJobCols,
		jobName, autoStart, restartOnComplete, intervalSeconds, uidVal(uid),
	))
	if err != nil {
		return nil, fmt.Errorf("upsertBackgroundJob %s: %w", jobName, err)
	}
	return j, nil
}

// EnsureRow inserts a default row for (user, jobName) if none exists. Used to
// seed entries the first time a user opens the Background Jobs panel.
func (r *BackgroundJobRepo) EnsureRow(ctx context.Context, jobName string, defaultIntervalSeconds int64) (*model.BackgroundJob, error) {
	existing, err := r.GetByName(ctx, jobName)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	uid := uidFromCtx(ctx)
	doNothing := `ON CONFLICT (user_id, job_name) DO NOTHING`
	if sqlutil.IsSQLite(ctx, r.pool) {
		doNothing = `ON CONFLICT (user_id, job_name) WHERE user_id IS NOT NULL DO NOTHING`
		if uid == 0 {
			doNothing = `ON CONFLICT (job_name) WHERE user_id IS NULL DO NOTHING`
		}
	}
	if _, err := r.pool.ExecContext(ctx,
		`INSERT INTO background_jobs (job_name, auto_start, restart_on_complete, interval_seconds, user_id)
		 VALUES (?1, ?2, ?3, ?4, ?5)
		 `+doNothing,
		jobName, false, false, defaultIntervalSeconds, uidVal(uid),
	); err != nil {
		return nil, fmt.Errorf("ensureBackgroundJob %s: %w", jobName, err)
	}
	return r.GetByName(ctx, jobName)
}

// SetAutoStart updates only the auto_start flag for (user, jobName).
func (r *BackgroundJobRepo) SetAutoStart(ctx context.Context, jobName string, autoStart bool) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE background_jobs
	      SET auto_start = ?1, updated_at = CURRENT_TIMESTAMP
	      WHERE job_name = ?2`
	args := []any{autoStart, jobName}
	q, args = addUIDFilterDollar(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("setBackgroundJobAutoStart %s: %w", jobName, err)
	}
	return nil
}

// MarkRunning records that a job run has started for (user, jobName). Marks
// last_run_result='running' so the scheduler can detect a still-active run.
// Pass uid==0 from a privileged scheduler context to update without filter; in
// that case caller MUST also pass userID.
func (r *BackgroundJobRepo) MarkRunning(ctx context.Context, userID int64, jobName string) error {
	_, err := r.pool.ExecContext(ctx,
		`UPDATE background_jobs
		 SET last_run_at      = CURRENT_TIMESTAMP,
		     last_run_result  = 'running',
		     last_run_message = NULL,
		     next_due_at      = NULL,
		     updated_at       = CURRENT_TIMESTAMP
		 WHERE job_name = ?1 AND user_id = ?2`,
		jobName, userID,
	)
	if err != nil {
		return fmt.Errorf("markBackgroundJobRunning %s: %w", jobName, err)
	}
	return nil
}

// MarkCompleted records the outcome of a job run for (userID, jobName) and sets
// next_due_at if provided. Used by the scheduler with a privileged context.
func (r *BackgroundJobRepo) MarkCompleted(ctx context.Context, userID int64, jobName, result, message string, nextDueAt *time.Time) error {
	var due any
	if nextDueAt != nil {
		due = nextDueAt.UTC().Format(time.RFC3339)
	} else {
		due = nil
	}
	_, err := r.pool.ExecContext(ctx,
		`UPDATE background_jobs
		 SET last_run_at      = CURRENT_TIMESTAMP,
		     last_run_result  = ?1,
		     last_run_message = ?2,
		     next_due_at      = ?3,
		     updated_at       = CURRENT_TIMESTAMP
		 WHERE job_name = ?4 AND user_id = ?5`,
		result, message, due, jobName, userID,
	)
	if err != nil {
		return fmt.Errorf("markBackgroundJobCompleted %s: %w", jobName, err)
	}
	return nil
}

// AllRows returns every row across all users (no uid filter). Intended for the
// scheduler goroutine, which needs to enumerate per-user schedules; never call
// this from a request-handling code path.
func (r *BackgroundJobRepo) AllRows(ctx context.Context) ([]*model.BackgroundJob, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT `+backgroundJobCols+` FROM background_jobs`,
	)
	if err != nil {
		return nil, fmt.Errorf("allBackgroundJobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.BackgroundJob
	for rows.Next() {
		j, err := scanBackgroundJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

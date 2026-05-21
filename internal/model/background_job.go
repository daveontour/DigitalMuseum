package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// BackgroundJob is a row from the background_jobs table — per-user scheduling
// state for a named maintenance job (thumbnails, embeddings backfills, etc.).
type BackgroundJob struct {
	ID                int64              `json:"id"`
	JobName           string             `json:"job_name"`
	AutoStart         bool               `json:"auto_start"`
	RestartOnComplete bool               `json:"restart_on_complete"`
	IntervalSeconds   int64              `json:"interval_seconds"`
	LastRunAt         sqlutil.NullDBTime `json:"last_run_at"`
	LastRunResult     *string            `json:"last_run_result"`
	LastRunMessage    *string            `json:"last_run_message"`
	NextDueAt         sqlutil.NullDBTime `json:"next_due_at"`
	CreatedAt         sqlutil.DBTime     `json:"created_at"`
	UpdatedAt         sqlutil.DBTime     `json:"updated_at"`
	UserID            *int64             `json:"user_id,omitempty"`
}

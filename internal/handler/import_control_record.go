package handler

import (
	"context"
	"database/sql"
)

// recordImportControlLastRun stores a per-user import job completion row for UI last-run display.
func recordImportControlLastRun(ctx context.Context, pool *sql.DB, uid int64, importType, result, msg string) error {
	if pool == nil || uid == 0 || importType == "" {
		return nil
	}
	if _, err := pool.ExecContext(ctx,
		`DELETE FROM import_control_last_run WHERE import_type = ? AND user_id = ?`,
		importType, uid,
	); err != nil {
		return err
	}
	_, err := pool.ExecContext(ctx,
		`INSERT INTO import_control_last_run (import_type, last_run_at, result, result_message, user_id)
		 VALUES (?, CURRENT_TIMESTAMP, ?, ?, ?)`,
		importType, result, msg, uid,
	)
	return err
}

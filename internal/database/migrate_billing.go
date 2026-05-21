package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// MigrateBilling creates tables for the billing database (LLM usage events only).
func MigrateBilling(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	for _, stmt := range billingSchemaSQLite() {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			preview := stmt
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			return fmt.Errorf("billing migration failed (%s): %w", preview, err)
		}
	}
	if err := migrateBillingArchiveProfilesIsDefault(ctx, db); err != nil {
		return fmt.Errorf("billing migration is_default column: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE llm_usage_events SET created_at = (datetime('now'))
		WHERE typeof(created_at) = 'text' AND created_at IN ('CURRENT_TEXT', 'TEXTTZ')`); err != nil {
		return fmt.Errorf("repair billing created_at: %w", err)
	}
	slog.Info("billing database migration complete")
	return nil
}

func billingSchemaSQLite() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS llm_usage_events (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at        TEXT NOT NULL DEFAULT (datetime('now')),
			provider          TEXT NOT NULL,
			user_id           INTEGER,
			is_visitor        INTEGER NOT NULL DEFAULT 0,
			input_tokens      INTEGER NOT NULL,
			output_tokens     INTEGER NOT NULL,
			model_name        TEXT,
			user_email        TEXT,
			user_first_name   TEXT,
			user_family_name  TEXT,
			used_server_llm_key INTEGER,
			succeeded         INTEGER NOT NULL DEFAULT 1,
			error_message     TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_events_created_at ON llm_usage_events (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_events_user_created ON llm_usage_events (user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS archive_profiles (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			username   TEXT,
			db_path    TEXT NOT NULL UNIQUE,
			enabled    INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			last_used  TEXT,
			is_default INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_profiles_enabled ON archive_profiles (enabled)`,
	}
}

// migrateBillingArchiveProfilesIsDefault adds is_default to archive_profiles on
// billing DBs created before that column existed.
func migrateBillingArchiveProfilesIsDefault(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('archive_profiles') WHERE name = 'is_default'`,
	).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx,
		`ALTER TABLE archive_profiles ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0`)
	return err
}

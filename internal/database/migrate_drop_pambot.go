package database

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
)

// dropPamBotSchema removes Pam Bot tables and the pam_bot_instructions column.
// Idempotent: tolerates missing tables/columns on repeat runs.
func dropPamBotSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	drops := []string{
		`DROP TABLE IF EXISTS pam_bot_turns`,
		`DROP TABLE IF EXISTS pam_bot_subjects`,
		`DROP TABLE IF EXISTS pam_bot_sessions`,
	}
	for _, stmt := range drops {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE app_system_instructions DROP COLUMN pam_bot_instructions`,
	); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "no such column") && !strings.Contains(msg, "duplicate column") {
			return err
		}
	}
	slog.Info("sqlite migration: dropped pam bot schema")
	return nil
}

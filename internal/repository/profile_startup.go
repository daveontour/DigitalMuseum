package repository

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ResolveMainSQLiteStartupPath picks the main SQLite file to open at process start
// from billing archive_profiles only (never from SQLITE_PATH).
// If exactly one enabled profile exists, it is marked is_default when none is set.
// Otherwise the enabled row with is_default=1 is used when its db_path exists on disk.
func ResolveMainSQLiteStartupPath(ctx context.Context, r *ProfileRepo) string {
	if err := r.PromoteSoleEnabledArchiveToDefault(ctx); err != nil {
		slog.Warn("startup main sqlite: promote sole archive default", "err", err)
	}

	if p, err := r.GetEnabledStartupDefault(ctx); err == nil && p != nil {
		dbAbs := normalizeMainDBPath(p.DBPath)
		_, statErr := os.Stat(dbAbs)
		if statErr == nil {
			slog.Info("startup main sqlite: using billing default archive profile", "db_path", dbAbs, "profile_id", p.ID)
			return dbAbs
		}
		// New Electron login flow: billing row + path exist before the SQLite file is
		// created. database/sqlite will create the file on first open; do not stay in
		// profiles-only mode (which breaks POST /auth/register after create-profile).
		if os.IsNotExist(statErr) {
			slog.Info("startup main sqlite: using billing default (archive file will be created on open)",
				"db_path", dbAbs, "profile_id", p.ID)
			return dbAbs
		}
		slog.Warn("startup main sqlite: billing default archive path not accessible",
			"db_path", dbAbs, "err", statErr)
	}
	slog.Info("startup main sqlite: no usable default archive — profiles-only mode")
	return ""
}

func normalizeMainDBPath(p string) string {
	clean := filepath.Clean(strings.TrimSpace(p))
	if abs, err := filepath.Abs(clean); err == nil {
		return abs
	}
	return clean
}

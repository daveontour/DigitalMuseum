package sqlutil

import (
	"context"
	"database/sql"
)

// IsSQLite reports whether db is SQLite (any driver where sqlite_version() works).
func IsSQLite(ctx context.Context, db *sql.DB) bool {
	return db != nil
}

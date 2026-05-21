package sqlutil

import (
	"context"
	"database/sql"
	"fmt"
)

// Vec0Upsert stores one row in a sqlite-vec vec0 virtual table keyed by rowid.
// table must be a trusted identifier from application constants (not user input).
//
// Uses DELETE then INSERT because INSERT OR REPLACE fails on vec0 tables when the
// row already exists (UNIQUE constraint on primary key). See sqlite-vec issue #259.
func Vec0Upsert(ctx context.Context, db *sql.DB, table string, rowid int64, embedding []byte, intIDs string) error {
	if db == nil {
		return fmt.Errorf("vec0 upsert: nil db")
	}
	if table == "" {
		return fmt.Errorf("vec0 upsert: empty table name")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE rowid = ?1`, rowid); err != nil {
		return fmt.Errorf("vec0 delete %s rowid %d: %w", table, rowid, err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO `+table+` (rowid, embedding, int_ids) VALUES (?1, ?2, ?3)`,
		rowid, embedding, intIDs,
	); err != nil {
		return fmt.Errorf("vec0 insert %s rowid %d: %w", table, rowid, err)
	}
	return nil
}

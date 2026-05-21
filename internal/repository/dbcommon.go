package repository

import (
	"database/sql"
	"errors"
)

// isNoRows reports whether err is sql.ErrNoRows (no row in result set).
func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// rowsAffectedOrZero returns sql.Result.RowsAffected, or 0 if the driver reports an error.
func rowsAffectedOrZero(r sql.Result) int64 {
	n, err := r.RowsAffected()
	if err != nil {
		return 0
	}
	return n
}

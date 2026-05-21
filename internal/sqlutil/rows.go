package sqlutil

import "database/sql"

// RowsAffected returns the result of sql.Result.RowsAffected, or 0 if unavailable.
func RowsAffected(r sql.Result) int64 {
	n, err := r.RowsAffected()
	if err != nil {
		return 0
	}
	return n
}

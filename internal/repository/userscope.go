package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/aimuseum/internal/appctx"
)

// uidFromCtx returns the authenticated user ID from context, or 0.
func uidFromCtx(ctx context.Context) int64 {
	return appctx.UserIDFromCtx(ctx)
}

// addUIDFilter appends "AND user_id = ?" to q+args when uid > 0.
// When uid == 0 (unauthenticated / single-tenant mode) no filter is added.
func addUIDFilter(q string, args []any, uid int64) (string, []any) {
	if uid == 0 {
		return q, args
	}
	args = append(args, uid)
	return q + " AND user_id = ?", args
}

// addUIDFilterDollar appends AND user_id = ?N using the next 1-based placeholder index (len(args)+1
// before uid is appended). Use for UPDATE/DELETE that already use ?1,?2,... (SQLite ?NNN indexed
// placeholders; go-sqlite3 does not bind $n by index when placeholders are out of textual order).
func addUIDFilterDollar(q string, args []any, uid int64) (string, []any) {
	if uid == 0 {
		return q, args
	}
	n := len(args) + 1
	args = append(args, uid)
	return fmt.Sprintf("%s AND user_id = ?%d", q, n), args
}

// addUIDFilterNullableGlobal appends "AND (user_id = ? OR user_id IS NULL)" when uid > 0.
// NULL user_id marks rows seeded for all tenants (JSON seed at startup); they are visible
// for list/get/exists but remain non-updatable via addUIDFilter on UPDATE/DELETE.
func addUIDFilterNullableGlobal(q string, args []any, uid int64) (string, []any) {
	if uid == 0 {
		return q, args
	}
	args = append(args, uid)
	return q + " AND (user_id = ? OR user_id IS NULL)", args
}

// addUIDFilterQualified appends "AND <alias>.user_id = ?" when uid > 0.
// Use when the query JOINs tables that each have user_id (multitenancy), e.g. media_items mi + post_media pm.
func addUIDFilterQualified(q string, args []any, uid int64, tableAlias string) (string, []any) {
	if uid == 0 {
		return q, args
	}
	args = append(args, uid)
	return q + fmt.Sprintf(" AND %s.user_id = ?", tableAlias), args
}

// uidVal returns the user_id value for INSERT statements.
// Returns nil (SQL NULL) for unauthenticated callers (uid==0).
func uidVal(uid int64) any {
	if uid == 0 {
		return nil
	}
	return uid
}

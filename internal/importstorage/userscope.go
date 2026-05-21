package importstorage

import (
	"context"

	"github.com/daveontour/aimuseum/internal/appctx"
)

// uidFromCtx extracts the user ID from the context (set by auth middleware).
// Returns 0 when no user is authenticated.
func uidFromCtx(ctx context.Context) int64 { return appctx.UserIDFromCtx(ctx) }

// uidVal converts a uid for use as a SQL parameter.
// Returns nil (SQL NULL) when uid == 0 (no authenticated user / single-tenant).
func uidVal(uid int64) any {
	if uid == 0 {
		return nil
	}
	return uid
}

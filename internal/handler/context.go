package handler

import (
	"context"

	appmiddleware "github.com/daveontour/aimuseum/internal/middleware"
)

// userIDFromCtx extracts the authenticated user's ID from the request context.
// Returns (0, false) when the request is unauthenticated.
//
// The value is injected by middleware.NewAuthMiddleware on every authenticated
// request. Handlers that require authentication should call this and return 401
// when ok is false.
func userIDFromCtx(ctx context.Context) (int64, bool) {
	return appmiddleware.UserIDFromCtx(ctx)
}

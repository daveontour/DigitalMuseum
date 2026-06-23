package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/service"
)

// UserIDFromCtx extracts the authenticated user's ID injected by AuthMiddleware.
// Returns (0, false) when the request is unauthenticated (exempt routes only).
func UserIDFromCtx(ctx context.Context) (int64, bool) {
	v := appctx.UserIDFromCtx(ctx)
	return v, v != 0
}

// exemptPrefixes lists path prefixes that never require authentication.
// Exact matches and prefix matches are both handled below.
var exemptPrefixes = []string{
	"/static/",
	"/share/",
	"/s/",
	"/visitor/",
	"/admin",        // admin panel has its own session-based auth
	"/api/profiles", // localhost-only archive profile endpoints; isLocalhost() guards in handler
}

// exemptExact lists paths that are always accessible without authentication.
var exemptExact = map[string]bool{
	"/health":                        true,
	"/favicon.ico":                   true,
	"/auth/login":                    true,
	"/auth/register":                 true,
	"/login":                         true,
	"/profiles":                      true,
	"/api/resolved-main-sqlite-path": true,
	"/api/local-ai/status":           true,
}

// NewAuthMiddleware returns a middleware that authenticates every request via
// the dm_session cookie and injects the user_id into the request context.
// Unauthenticated requests to non-exempt paths receive 401 JSON, or a redirect
// to /login when the client Accept header prefers HTML (browser navigation).
//
// Exempt from authentication:
//   - GET /health
//   - GET /favicon.ico
//   - GET /static/*
//   - GET /api/local-ai/status
//   - POST /auth/login
//   - POST /auth/register
//   - Paths under /share/, /s/, /visitor/, /admin (see isExempt)
func NewAuthMiddleware(svc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// ── Exempt paths pass straight through ───────────────────────────
			if isExempt(path) {
				next.ServeHTTP(w, r)
				return
			}

			// ── Read session cookie ──────────────────────────────────────────
			var sessionID string
			if c, err := r.Cookie(service.AuthSessionCookieName); err == nil && c != nil {
				sessionID = c.Value
			}

			// ── Authenticate ─────────────────────────────────────────────────
			// Authenticate also slides the session TTL forward on each hit.
			auth, err := svc.Authenticate(r.Context(), sessionID)
			if err != nil {
				slog.Error("authenticate failed", "err", err, "path", path)
				writeAuthError(w, http.StatusInternalServerError, "session lookup failed")
				return
			}

			if auth == nil || auth.User == nil {
				if looksLikeBrowserNav(r) {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				writeAuthError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			// ── Inject user_id, visitor flag, and per-feature access into context ──
			ctx := context.WithValue(r.Context(), appctx.ContextKeyUserID, auth.User.ID)
			ctx = context.WithValue(ctx, appctx.ContextKeyIsVisitor, auth.IsVisitor)
			ctx = context.WithValue(ctx, appctx.ContextKeyVisitorAccess, auth.Access)
			if auth.VisitorKeyHintID != nil {
				ctx = context.WithValue(ctx, appctx.ContextKeyVisitorKeyHintID, *auth.VisitorKeyHintID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isExempt returns true if the path does not require authentication.
func isExempt(path string) bool {
	if exemptExact[path] {
		return true
	}
	for _, prefix := range exemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// looksLikeBrowserNav returns true when the request is a top-level browser
// navigation (Accept header prefers text/html) rather than an API/XHR call.
func looksLikeBrowserNav(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// writeAuthError writes a JSON error response consistent with the rest of the API.
func writeAuthError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  detail,
		"status": status,
	})
}

// Package appctx defines shared context keys used across packages
// (middleware, crypto, repository) without creating import cycles.
package appctx

import "context"

// contextKey is a private type for this package's context keys.
type contextKey int

const (
	// ContextKeyUserID is the context key for the authenticated user's ID (int64).
	// Injected by middleware.NewAuthMiddleware on every authenticated request.
	// Read via UserIDFromCtx.
	ContextKeyUserID contextKey = iota

	// ContextKeyIsVisitor marks sessions created via visitor key login (bool).
	// Injected alongside ContextKeyUserID by the auth middleware.
	// Read via IsVisitorFromCtx.
	ContextKeyIsVisitor

	// ContextKeyVisitorAccess carries per-feature gates for restricted visitor-key sessions.
	// Injected by auth middleware when a user session is present.
	ContextKeyVisitorAccess

	// ContextKeyVisitorKeyHintID is visitor_key_hints.id for visitor-key login sessions that
	// linked a hint row. Omitted for owner login, share visitors, or hint-less visitor sessions.
	// Stored value type is int64. Read via VisitorKeyHintIDFromCtx.
	ContextKeyVisitorKeyHintID
)

// VisitorAccess describes which archive areas a visitor-key session may use.
// When Restricted is false (owner login or share-link visitor), all Allow* methods return true.
type VisitorAccess struct {
	Restricted          bool
	CanMessagesChat     bool
	CanEmails           bool
	CanContacts         bool
	CanRelationships    bool // relationship graph, Profiles / complete-profile APIs
	CanSensitivePrivate bool // Sensitive Data gallery and sensitive-reference AI tools
}

// AllowMessagesChat is true for owners, share visitors, or visitor keys with the flag set.
func (a VisitorAccess) AllowMessagesChat() bool {
	if !a.Restricted {
		return true
	}
	return a.CanMessagesChat
}

// AllowEmails is true when the session may use email-related UI and APIs.
func (a VisitorAccess) AllowEmails() bool {
	if !a.Restricted {
		return true
	}
	return a.CanEmails
}

// AllowContacts is true when the session may use the contacts directory (not the relationship graph).
func (a VisitorAccess) AllowContacts() bool {
	if !a.Restricted {
		return true
	}
	return a.CanContacts
}

// AllowRelationships is true for the relationship graph and contact profile / complete-profile features.
func (a VisitorAccess) AllowRelationships() bool {
	if !a.Restricted {
		return true
	}
	return a.CanRelationships
}

// AllowSensitivePrivate is true for the Sensitive Data gallery and sensitive reference-document AI tools.
func (a VisitorAccess) AllowSensitivePrivate() bool {
	if !a.Restricted {
		return true
	}
	return a.CanSensitivePrivate
}

// VisitorAccessFromCtx returns access flags injected by auth middleware.
// When absent (unauthenticated request), returns a non-restricted value so legacy behaviour is unchanged.
func VisitorAccessFromCtx(ctx context.Context) VisitorAccess {
	v, ok := ctx.Value(ContextKeyVisitorAccess).(VisitorAccess)
	if !ok {
		return VisitorAccess{Restricted: false}
	}
	return v
}

// UserIDFromCtx extracts the authenticated user's ID from the request context.
// Returns 0 when the request is unauthenticated or the middleware is running
// in non-enforcing mode and no valid session was found.
func UserIDFromCtx(ctx context.Context) int64 {
	v, _ := ctx.Value(ContextKeyUserID).(int64)
	return v
}

// IsVisitorFromCtx returns true when the current session was created via visitor
// key login (as opposed to a normal user login or registration).
func IsVisitorFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ContextKeyIsVisitor).(bool)
	return v
}

// VisitorKeyHintIDFromCtx returns the visitor_key_hints row id when the auth
// middleware recorded one (visitor-key session with a linked hint).
func VisitorKeyHintIDFromCtx(ctx context.Context) (hintID int64, ok bool) {
	v, typed := ctx.Value(ContextKeyVisitorKeyHintID).(int64)
	return v, typed
}

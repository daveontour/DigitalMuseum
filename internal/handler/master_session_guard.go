package handler

import (
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
)

// OwnerMasterUnlockOK is true when either:
//   - the browser session has owner master key material in RAM (not a visitor seat), or
//   - there is no sensitive keyring in this deployment (e.g. SQLite build) and the user is
//     the authenticated archive owner (not a visitor / share session).
//
// Use for import/upload paths that must match GET /api/session/master-key/status "master_unlocked".
func OwnerMasterUnlockOK(r *http.Request, sessionStore *keystore.SessionMasterStore, sensitiveSvc *service.SensitiveService, authSvc *service.AuthService) bool {
	if sessionStore == nil {
		return false
	}
	unlocked, master := sessionStore.SessionStatus(r)
	if unlocked && master {
		return true
	}
	if sensitiveSvc == nil || authSvc == nil {
		return false
	}
	n, err := sensitiveSvc.KeyCount(r.Context())
	if err != nil || n > 0 {
		return false
	}
	var sid string
	if c, err := r.Cookie(service.AuthSessionCookieName); err == nil && c != nil {
		sid = strings.TrimSpace(c.Value)
	}
	if sid == "" {
		return false
	}
	auth, err := authSvc.Authenticate(r.Context(), sid)
	return err == nil && auth != nil && auth.User != nil && !auth.IsVisitor
}

// ArchiveOwnerAuthenticated is true when the session cookie identifies a logged-in
// archive owner (not a visitor / share key seat). It does not imply the master key
// is unlocked in RAM — use for read-only owner UI when decryption is optional.
func ArchiveOwnerAuthenticated(r *http.Request, authSvc *service.AuthService) bool {
	if authSvc == nil {
		return false
	}
	var sid string
	if c, err := r.Cookie(service.AuthSessionCookieName); err == nil && c != nil {
		sid = strings.TrimSpace(c.Value)
	}
	if sid == "" {
		return false
	}
	auth, err := authSvc.Authenticate(r.Context(), sid)
	return err == nil && auth != nil && auth.User != nil && !auth.IsVisitor
}

// RequireOwnerMasterUnlockOrNoKeyring is like RequireOwnerMasterUnlock but allows archive owners
// when no keyring exists (nothing to unlock). See OwnerMasterUnlockOK.
func RequireOwnerMasterUnlockOrNoKeyring(w http.ResponseWriter, r *http.Request, sessionStore *keystore.SessionMasterStore, sensitiveSvc *service.SensitiveService, authSvc *service.AuthService) bool {
	if OwnerMasterUnlockOK(r, sessionStore, sensitiveSvc, authSvc) {
		return true
	}
	if sessionStore == nil {
		writeError(w, http.StatusForbidden, "owner master key unlock required in this browser session to modify or delete data")
		return false
	}
	writeError(w, http.StatusForbidden, "owner master key unlock required — visitor sessions cannot modify or delete data")
	return false
}

// RequireOwnerMasterUnlock writes 403 and returns false unless this browser session was
// unlocked with the owner master key (not a visitor keyring seat). Use on endpoints
// that modify or delete archive data.
func RequireOwnerMasterUnlock(w http.ResponseWriter, r *http.Request, sessionStore *keystore.SessionMasterStore) bool {
	if sessionStore == nil {
		writeError(w, http.StatusForbidden, "owner master key unlock required in this browser session to modify or delete data")
		return false
	}
	unlocked, master := sessionStore.SessionStatus(r)
	if !unlocked || !master {
		writeError(w, http.StatusForbidden, "owner master key unlock required — visitor sessions cannot modify or delete data")
		return false
	}
	return true
}

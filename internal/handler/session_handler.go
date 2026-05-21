package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// SessionHandler exposes session-scoped endpoints (per-browser master key unlock).
type SessionHandler struct {
	sensitiveSvc *service.SensitiveService
	sessionStore *keystore.SessionMasterStore
	authSvc      *service.AuthService
}

// NewSessionHandler constructs a SessionHandler.
func NewSessionHandler(sensitiveSvc *service.SensitiveService, sessionStore *keystore.SessionMasterStore, authSvc *service.AuthService) *SessionHandler {
	return &SessionHandler{sensitiveSvc: sensitiveSvc, sessionStore: sessionStore, authSvc: authSvc}
}

// RegisterRoutes mounts /api/session/* routes.
func (h *SessionHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/session/master-key/status", h.MasterKeyStatus)
	r.Post("/api/session/master-key/unlock", h.MasterKeyUnlock)
	r.Post("/api/session/master-key/unlock-visitor", h.MasterKeyUnlockVisitor)
	r.Post("/api/session/master-key/clear", h.MasterKeyClear)
	r.Get("/api/session/visitor-key-hints", h.VisitorKeyHintsList)
}

// VisitorKeyHintsList returns plain-text hints for visitor (non-master) keyring seats.
func (h *SessionHandler) VisitorKeyHintsList(w http.ResponseWriter, r *http.Request) {
	hints, err := h.sensitiveSvc.ListVisitorKeyHints(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing visitor key hints: %s", err))
		return
	}
	// Encode with explicit timestamps for JSON
	out := make([]map[string]any, 0, len(hints))
	for _, x := range hints {
		out = append(out, map[string]any{
			"id":         x.ID,
			"hint":       x.Hint,
			"created_at": x.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, map[string]any{"hints": out})
}

// MasterKeyClear drops this browser's session master key.
func (h *SessionHandler) MasterKeyClear(w http.ResponseWriter, r *http.Request) {
	h.sessionStore.Clear(w, r)
	writeJSON(w, map[string]any{"cleared": true})
}

// MasterKeyStatus reports whether a keyring exists and whether this browser session has unlock material.
// master_unlocked is true only when the owner master key was used (visitor seat unlock sets unlocked but not master_unlocked).
// When no keyring exists (e.g. SQLite build without encryption tables), a logged-in archive owner is treated as
// master_unlocked for UI gating — there is no key material to unlock.
func (h *SessionHandler) MasterKeyStatus(w http.ResponseWriter, r *http.Request) {
	n, err := h.sensitiveSvc.KeyCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reading keyring: %s", err))
		return
	}
	keyringConfigured := n > 0
	unlocked, masterUnlocked := h.sessionStore.SessionStatus(r)
	if !keyringConfigured && h.authSvc != nil {
		var sid string
		if c, err := r.Cookie(service.AuthSessionCookieName); err == nil && c != nil {
			sid = strings.TrimSpace(c.Value)
		}
		if sid != "" {
			if auth, err := h.authSvc.Authenticate(r.Context(), sid); err == nil && auth != nil && auth.User != nil && !auth.IsVisitor {
				masterUnlocked = true
			}
		}
	}
	writeJSON(w, map[string]any{
		"keyring_configured": keyringConfigured,
		"unlocked":           unlocked,
		"master_unlocked":    masterUnlocked,
	})
}

// MasterKeyUnlock validates the password against the master keyring row and stores it in RAM only.
func (h *SessionHandler) MasterKeyUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	ok, err := h.sensitiveSvc.VerifyMasterPassword(r.Context(), req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error validating key: %s", err))
		return
	}
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(w, map[string]any{
			"valid":  false,
			"detail": "That master key does not match the keyring. Try again or skip.",
		})
		return
	}
	if err := h.sessionStore.Put(w, r, req.Password, true); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("session error: %s", err))
		return
	}
	writeJSON(w, map[string]any{"valid": true})
}

// MasterKeyUnlockVisitor validates a non-master keyring seat password, rejects the owner master key,
// and stores the password in RAM (same slot as master unlock) for shared DEK operations.
func (h *SessionHandler) MasterKeyUnlockVisitor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	okMaster, err := h.sensitiveSvc.VerifyMasterPassword(r.Context(), req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error validating key: %s", err))
		return
	}
	if okMaster {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{
			"valid":  false,
			"detail": "Invalid visitor key, or no visitor keys are set up. Ask the owner to add a seat in Manage Keys.",
		})
		return
	}
	okVisitor, err := h.sensitiveSvc.VerifyVisitorKeyringPassword(r.Context(), req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error validating visitor key: %s", err))
		return
	}
	if !okVisitor {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(w, map[string]any{
			"valid":  false,
			"detail": "Invalid visitor key, or no visitor keys are set up. Ask the owner to add a seat in Manage Keys.",
		})
		return
	}
	if err := h.sessionStore.Put(w, r, req.Password, false); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("session error: %s", err))
		return
	}
	writeJSON(w, map[string]any{"valid": true, "visitor": true})
}

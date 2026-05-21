package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ShareHandler manages archive share tokens (owner CRUD) and the visitor join flow.
type ShareHandler struct {
	svc          *service.ArchiveShareService
	secure       bool // mirrors SessionCookieSecure for the dm_session cookie
	sessionStore *keystore.SessionMasterStore
}

// NewShareHandler creates a ShareHandler.
func NewShareHandler(svc *service.ArchiveShareService, secure bool, sessionStore *keystore.SessionMasterStore) *ShareHandler {
	return &ShareHandler{svc: svc, secure: secure, sessionStore: sessionStore}
}

// RegisterRoutes mounts share endpoints.
// Owner endpoints require authentication (handled by middleware).
// Visitor endpoints (/share/*) are exempt from auth (see middleware/auth.go exemptPrefixes).
func (h *ShareHandler) RegisterRoutes(r chi.Router) {
	// ── Owner endpoints (authenticated) ──────────────────────────────────────
	r.Post("/api/shares", h.Create)
	r.Get("/api/shares", h.List)
	r.Delete("/api/shares/{token}", h.Delete)

	// ── Visitor endpoints (exempt from auth middleware) ───────────────────────
	r.Get("/share/{token}", h.GetInfo)
	r.Post("/share/{token}", h.Join)
}

// ── Owner handlers ────────────────────────────────────────────────────────────

// POST /api/shares
// Body: { "label": "...", "password": "...", "expires_at": "2026-01-01T00:00:00Z", "tool_access_policy": "{...}" }
// Returns 201 + { "id": "...", ... }
func (h *ShareHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	uid, ok := userIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	_ = uid // authSvc.CreateShareSession uses ctx directly

	var req struct {
		Label            string  `json:"label"`
		Password         string  `json:"password"`
		ExpiresAt        *string `json:"expires_at"`
		ToolAccessPolicy string  `json:"tool_access_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "expires_at must be RFC3339 format")
			return
		}
		expiresAt = &t
	}

	share, err := h.svc.CreateShare(r.Context(), req.Label, req.Password, expiresAt, req.ToolAccessPolicy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create share failed")
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, share)
}

// GET /api/shares
// Returns the authenticated user's share tokens.
func (h *ShareHandler) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := userIDFromCtx(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	shares, err := h.svc.ListShares(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list shares failed")
		return
	}
	if shares == nil {
		shares = []*model.ArchiveShare{}
	}
	writeJSON(w, shares)
}

// DELETE /api/shares/{token}
// Removes a share token owned by the authenticated user.
func (h *ShareHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if _, ok := userIDFromCtx(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	token := chi.URLParam(r, "token")
	if err := h.svc.DeleteShare(r.Context(), token); err != nil {
		writeError(w, http.StatusInternalServerError, "delete share failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Visitor handlers ──────────────────────────────────────────────────────────

// GET /share/{token}
// Returns public metadata about a share token so the frontend can decide whether
// to show a password form. Does not require authentication.
// Response: { "id", "label", "has_password", "expires_at", "owner_display_name" }
func (h *ShareHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	pub, err := h.svc.GetSharePublic(r.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrShareNotFound):
			writeError(w, http.StatusNotFound, "share token not found")
		case errors.Is(err, service.ErrShareExpired):
			writeError(w, http.StatusGone, "share token has expired")
		default:
			writeError(w, http.StatusInternalServerError, "look up share failed")
		}
		return
	}
	writeJSON(w, pub)
}

// POST /share/{token}
// Body: { "password": "..." }  (password may be omitted for open shares)
// Validates the share token, creates a visitor session scoped to the archive owner,
// sets the dm_session cookie, and returns { "session_created": true }.
func (h *ShareHandler) Join(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var req struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body may be empty for open shares

	sessionID, err := h.svc.JoinShare(r.Context(), token, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrShareNotFound):
			writeError(w, http.StatusNotFound, "share token not found")
		case errors.Is(err, service.ErrShareExpired):
			writeError(w, http.StatusGone, "share token has expired")
		case errors.Is(err, service.ErrSharePasswordRequired):
			writeError(w, http.StatusUnauthorized, "password required")
		case errors.Is(err, service.ErrSharePasswordInvalid):
			writeError(w, http.StatusUnauthorized, "incorrect password")
		default:
			writeError(w, http.StatusInternalServerError, "join share failed")
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     service.AuthSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   service.CookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secure,
	})
	writeJSON(w, map[string]any{"session_created": true})
}

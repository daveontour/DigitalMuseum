package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// SessionHandler exposes session-scoped endpoints (in-RAM master key unlock).
type SessionHandler struct {
	sensitiveSvc *service.SensitiveService
	ramMaster    *keystore.MemoryMasterKey
}

// NewSessionHandler constructs a SessionHandler.
func NewSessionHandler(sensitiveSvc *service.SensitiveService, ram *keystore.MemoryMasterKey) *SessionHandler {
	return &SessionHandler{sensitiveSvc: sensitiveSvc, ramMaster: ram}
}

// RegisterRoutes mounts /api/session/* routes.
func (h *SessionHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/session/master-key/status", h.MasterKeyStatus)
	r.Post("/api/session/master-key/unlock", h.MasterKeyUnlock)
}

// MasterKeyStatus reports whether a keyring exists and whether this process has an unlocked master key in RAM.
func (h *SessionHandler) MasterKeyStatus(w http.ResponseWriter, r *http.Request) {
	n, err := h.sensitiveSvc.KeyCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reading keyring: %s", err))
		return
	}
	_, unlocked := h.ramMaster.Get()
	writeJSON(w, map[string]any{
		"keyring_configured": n > 0,
		"unlocked":           unlocked,
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
	h.ramMaster.Set(req.Password)
	writeJSON(w, map[string]any{"valid": true})
}

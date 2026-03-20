package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// PrivateStoreHandler handles all /private-store/* endpoints.
// Every endpoint requires the master password because entries are encrypted
// with the master-only private DEK.
type PrivateStoreHandler struct {
	svc          *service.PrivateStoreService
	sessionStore *keystore.SessionMasterStore
}

// NewPrivateStoreHandler creates a PrivateStoreHandler.
func NewPrivateStoreHandler(svc *service.PrivateStoreService, sessionStore *keystore.SessionMasterStore) *PrivateStoreHandler {
	return &PrivateStoreHandler{svc: svc, sessionStore: sessionStore}
}

// RegisterRoutes mounts all private-store routes.
// Fixed paths are registered before the parameterised /{key} route.
func (h *PrivateStoreHandler) RegisterRoutes(r chi.Router) {
	r.Get("/private-store", h.List)
	r.Post("/private-store", h.Create)
	r.Get("/private-store/{key}", h.GetByKey)
	r.Put("/private-store/{key}", h.Update)
	r.Delete("/private-store/{key}", h.Delete)
}

// masterPasswordFromRequest extracts the master password from the X-Master-Password header,
// ?master_password= query parameter, or the in-RAM unlock from this session.
func (h *PrivateStoreHandler) masterPasswordFromRequest(r *http.Request) string {
	if v := r.Header.Get("X-Master-Password"); strings.TrimSpace(v) != "" {
		return appcrypto.NormalizeKeyringPassword(v)
	}
	return resolveMasterPassword(r.URL.Query().Get("master_password"), r, h.sessionStore)
}

// List handles GET /private-store
// Master password: X-Master-Password header or ?master_password= query param.
func (h *PrivateStoreHandler) List(w http.ResponseWriter, r *http.Request) {
	mp := h.masterPasswordFromRequest(r)
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	entries, err := h.svc.List(r.Context(), mp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing entries: %s", err))
		return
	}
	writeJSON(w, entries)
}

// GetByKey handles GET /private-store/{key}
func (h *PrivateStoreHandler) GetByKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	mp := h.masterPasswordFromRequest(r)
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	entry, err := h.svc.GetByKey(r.Context(), key, mp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error getting entry: %s", err))
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("key %q not found", key))
		return
	}
	writeJSON(w, entry)
}

// Create handles POST /private-store
// Body: {"key":"...","value":"...","master_password":"..."}
func (h *PrivateStoreHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key            string `json:"key"`
		Value          string `json:"value"`
		MasterPassword string `json:"master_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	mp := resolveMasterPassword(req.MasterPassword, r, h.sessionStore)
	if strings.TrimSpace(mp) == "" {
		mp = h.masterPasswordFromRequest(r)
	}
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	if err := h.svc.Create(r.Context(), req.Key, req.Value, mp); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating entry: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"message": "entry created", "key": req.Key})
}

// Update handles PUT /private-store/{key}
// Body: {"value":"...","master_password":"..."}
func (h *PrivateStoreHandler) Update(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req struct {
		Value          string `json:"value"`
		MasterPassword string `json:"master_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mp := resolveMasterPassword(req.MasterPassword, r, h.sessionStore)
	if strings.TrimSpace(mp) == "" {
		mp = h.masterPasswordFromRequest(r)
	}
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	if err := h.svc.Update(r.Context(), key, req.Value, mp); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating entry: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "entry updated", "key": key})
}

// Delete handles DELETE /private-store/{key}
// Master password: X-Master-Password header, ?master_password= query param, or JSON body.
func (h *PrivateStoreHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	mp := h.masterPasswordFromRequest(r)
	if strings.TrimSpace(mp) == "" {
		// Try JSON body for DELETE requests that include a body.
		var req struct {
			MasterPassword string `json:"master_password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mp = resolveMasterPassword(req.MasterPassword, r, h.sessionStore)
	}
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	if err := h.svc.Delete(r.Context(), key, mp); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting entry: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "entry deleted", "key": key})
}

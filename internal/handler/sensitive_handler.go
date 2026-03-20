package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// SensitiveHandler handles all /sensitive-data/* endpoints.
type SensitiveHandler struct {
	svc             *service.SensitiveService
	pythonStaticDir string
	sessionStore    *keystore.SessionMasterStore
}

// NewSensitiveHandler creates a SensitiveHandler.
func NewSensitiveHandler(svc *service.SensitiveService, pythonStaticDir string, sessionStore *keystore.SessionMasterStore) *SensitiveHandler {
	return &SensitiveHandler{svc: svc, pythonStaticDir: pythonStaticDir, sessionStore: sessionStore}
}

// RegisterRoutes mounts all sensitive-data routes.
// Fixed paths are registered before the parameterised /{record_id} for correct matching.
func (h *SensitiveHandler) RegisterRoutes(r chi.Router) {
	r.Get("/sensitive-data/count", h.Count)
	r.Get("/sensitive-data/key-count", h.KeyCount)
	r.Get("/sensitive-data/hints", h.GetHints)
	r.Get("/sensitive-data", h.ListAll)
	r.Get("/sensitive-data/{record_id}", h.GetByID)

	r.Post("/sensitive-data/master-key", h.GenerateMasterKey)
	r.Post("/sensitive-data/trusted-key", h.GenerateTrustedKey)
	r.Delete("/sensitive-data/trusted-key", h.DeleteTrustedKey)

	r.Post("/sensitive-data", h.Create)
	r.Put("/sensitive-data/{record_id}", h.Update)
	r.Delete("/sensitive-data/{record_id}", h.DeleteRecord)
}

// ── read endpoints ────────────────────────────────────────────────────────────

func (h *SensitiveHandler) Count(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error counting records: %s", err))
		return
	}
	writeJSON(w, map[string]int64{"count": n})
}

func (h *SensitiveHandler) KeyCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.KeyCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error counting keys: %s", err))
		return
	}
	writeJSON(w, map[string]int64{"count": n})
}

func (h *SensitiveHandler) GetHints(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(h.pythonStaticDir, "data", "password_hints.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, map[string]any{"hints": []any{}})
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		writeJSON(w, map[string]any{"hints": []any{}})
		return
	}
	hints := parsed["hints"]
	if hints == nil {
		hints = []any{}
	}
	writeJSON(w, map[string]any{"hints": hints})
}

func (h *SensitiveHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	password := resolveMasterPassword(r.URL.Query().Get("password"), r, h.sessionStore)
	records, err := h.svc.ListAll(r.Context(), password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing records: %s", err))
		return
	}
	writeJSON(w, records)
}

func (h *SensitiveHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSensitiveID(w, r)
	if !ok {
		return
	}
	password := resolveMasterPassword(r.URL.Query().Get("password"), r, h.sessionStore)
	record, err := h.svc.GetByID(r.Context(), id, password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error getting record: %s", err))
		return
	}
	if record == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("record %d not found", id))
		return
	}
	writeJSON(w, record)
}

// ── key management ────────────────────────────────────────────────────────────

// GenerateMasterKey initialises a fresh pgcrypto keyring for the master password.
// Endpoint: POST /sensitive-data/master-key
// Body: {"password":"..."}
func (h *SensitiveHandler) GenerateMasterKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !sensitiveHasPassword(req.Password) {
		writeError(w, http.StatusForbidden, "password is required")
		return
	}
	if err := h.svc.GenerateKeyring(r.Context(), req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error initialising keyring: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"message": "Keyring initialised"})
}

// GenerateTrustedKey adds a new keyring seat for user_password using master_password.
// Endpoint: POST /sensitive-data/trusted-key
// Body: {"user_password":"...","master_password":"..."}
func (h *SensitiveHandler) GenerateTrustedKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserPassword   string `json:"user_password"`
		MasterPassword string `json:"master_password"`
		Hint           string `json:"hint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !sensitiveHasPassword(req.UserPassword) {
		writeError(w, http.StatusForbidden, "user password is required")
		return
	}
	if !sensitiveHasPassword(req.MasterPassword) {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	if strings.TrimSpace(req.Hint) == "" {
		writeError(w, http.StatusBadRequest, "hint is required (plain-text reminder shown on the unlock screen for visitors)")
		return
	}
	if err := h.svc.AddUser(r.Context(), req.UserPassword, req.MasterPassword, req.Hint); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error adding user: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"message": "Keyring seat added"})
}

// DeleteTrustedKey removes the keyring seat for user_password.
// Endpoint: DELETE /sensitive-data/trusted-key
// Body: {"user_password":"...","master_password":"..."}
func (h *SensitiveHandler) DeleteTrustedKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserPassword   string `json:"user_password"`
		MasterPassword string `json:"master_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !sensitiveHasPassword(req.UserPassword) {
		writeError(w, http.StatusForbidden, "user password is required")
		return
	}
	if !sensitiveHasPassword(req.MasterPassword) {
		writeError(w, http.StatusForbidden, "master password is required")
		return
	}
	if err := h.svc.RemoveUser(r.Context(), req.UserPassword, req.MasterPassword); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error removing user: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "Keyring seat removed"})
}

// ── write endpoints ───────────────────────────────────────────────────────────

func (h *SensitiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string `json:"description"`
		Details     string `json:"details"`
		IsPrivate   bool   `json:"is_private"`
		IsSensitive bool   `json:"is_sensitive"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pw := resolveMasterPassword(req.Password, r, h.sessionStore)
	if !sensitiveHasPassword(pw) {
		writeError(w, http.StatusForbidden, "a password is required to create records")
		return
	}
	if err := h.svc.Create(r.Context(), pw, req.Description, req.Details, req.IsPrivate, req.IsSensitive); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating record: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"message": "Record created in the database"})
}

func (h *SensitiveHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSensitiveID(w, r)
	if !ok {
		return
	}
	var req struct {
		Description string `json:"description"`
		Details     string `json:"details"`
		IsPrivate   bool   `json:"is_private"`
		IsSensitive bool   `json:"is_sensitive"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pw := resolveMasterPassword(req.Password, r, h.sessionStore)
	if !sensitiveHasPassword(pw) {
		writeError(w, http.StatusForbidden, "a password is required to update records")
		return
	}
	if err := h.svc.Update(r.Context(), id, pw, req.Description, req.Details, req.IsPrivate, req.IsSensitive); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating record: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "Record updated in the database"})
}

func (h *SensitiveHandler) DeleteRecord(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSensitiveID(w, r)
	if !ok {
		return
	}
	password := resolveMasterPassword(r.URL.Query().Get("password"), r, h.sessionStore)
	if !sensitiveHasPassword(password) {
		writeError(w, http.StatusForbidden, "a password is required to delete records")
		return
	}
	if err := h.svc.Delete(r.Context(), id, password); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting record: %s", err))
		return
	}
	writeJSON(w, map[string]any{"success": true, "message": "Record deleted from the database"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseSensitiveID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "record_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "record_id must be an integer")
		return 0, false
	}
	return id, true
}

func sensitiveHasPassword(s string) bool {
	return strings.TrimSpace(s) != ""
}

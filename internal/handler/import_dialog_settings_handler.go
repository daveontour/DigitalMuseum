package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// Legacy private_store key used before generic import_dialog_* keys (IMAP only).
const imapImportDialogLegacyPrivateKey = "imap_import_dialog_v1"

// ImportDialogSettingsHandler persists encrypted JSON blobs per import dialog kind
// in private_store (master-derived DEK; master password only from server RAM).
type ImportDialogSettingsHandler struct {
	privateStore *service.PrivateStoreService
	sessionStore *keystore.SessionMasterStore
	authSvc      *service.AuthService
}

// NewImportDialogSettingsHandler constructs the handler.
func NewImportDialogSettingsHandler(privateStore *service.PrivateStoreService, sessionStore *keystore.SessionMasterStore, authSvc *service.AuthService) *ImportDialogSettingsHandler {
	return &ImportDialogSettingsHandler{privateStore: privateStore, sessionStore: sessionStore, authSvc: authSvc}
}

// allowedImportDialogKinds maps URL segment -> private_store key suffix (import_dialog_<kind>_v1).
var allowedImportDialogKinds = map[string]string{
	"imap":                 "imap",
	"gmail":                "gmail",
	"imessage":             "imessage",
	"whatsapp":             "whatsapp",
	"facebook_all":         "facebook_all",
	"instagram":            "instagram",
	"filesystem":           "filesystem",
	"filesystem_reference": "filesystem_reference",
}

func importDialogStoreKey(kind string) string {
	return "import_dialog_" + kind + "_v1"
}

// RegisterRoutes mounts GET/PUT /api/import-saved-settings/{kind}.
func (h *ImportDialogSettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/import-saved-settings/{kind}", h.Get)
	r.Put("/api/import-saved-settings/{kind}", h.Put)
}

func (h *ImportDialogSettingsHandler) masterPasswordOrRespond(w http.ResponseWriter, r *http.Request, forPut bool) (string, bool) {
	if h.sessionStore == nil {
		if forPut {
			writeError(w, http.StatusForbidden, "Master key is not unlocked in this session.")
		} else {
			writeJSON(w, map[string]any{"ok": false, "reason": "master_key_not_unlocked"})
		}
		return "", false
	}
	mp, ok := h.sessionStore.Get(r)
	if !ok || mp == "" {
		if forPut {
			writeError(w, http.StatusForbidden, "Master key is not unlocked in this session.")
		} else {
			writeJSON(w, map[string]any{"ok": false, "reason": "master_key_not_unlocked"})
		}
		return "", false
	}
	if h.privateStore == nil {
		writeError(w, http.StatusInternalServerError, "private store not configured")
		return "", false
	}
	return mp, true
}

// Get handles GET /api/import-saved-settings/{kind}.
func (h *ImportDialogSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	kindSeg := chi.URLParam(r, "kind")
	storedKind, ok := allowedImportDialogKinds[kindSeg]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown import dialog kind")
		return
	}
	var mp string
	if h.sessionStore != nil {
		if p, have := h.sessionStore.Get(r); have && p != "" {
			mp = p
		}
	}
	if mp == "" {
		if ArchiveOwnerAuthenticated(r, h.authSvc) {
			writeJSON(w, map[string]any{"ok": true, "saved": false})
			return
		}
		writeJSON(w, map[string]any{"ok": false, "reason": "master_key_not_unlocked"})
		return
	}
	if h.privateStore == nil {
		writeError(w, http.StatusInternalServerError, "private store not configured")
		return
	}
	ctx := r.Context()
	key := importDialogStoreKey(storedKind)
	rec, err := h.privateStore.GetByKey(ctx, key, mp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load saved import settings: %s", err))
		return
	}
	if rec == nil && storedKind == "imap" {
		rec, err = h.privateStore.GetByKey(ctx, imapImportDialogLegacyPrivateKey, mp)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("load saved import settings: %s", err))
			return
		}
	}
	if rec == nil {
		writeJSON(w, map[string]any{"ok": true, "saved": false})
		return
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(rec.Value), &settings); err != nil {
		writeJSON(w, map[string]any{"ok": true, "saved": false, "reason": "invalid_stored_data"})
		return
	}
	if settings == nil {
		settings = map[string]any{}
	}
	writeJSON(w, map[string]any{"ok": true, "saved": true, "settings": settings})
}

// Put handles PUT /api/import-saved-settings/{kind} with a JSON object body.
func (h *ImportDialogSettingsHandler) Put(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	kindSeg := chi.URLParam(r, "kind")
	storedKind, ok := allowedImportDialogKinds[kindSeg]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown import dialog kind")
		return
	}
	mp, ok := h.masterPasswordOrRespond(w, r, true)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	m, ok := payload.(map[string]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "body must be a JSON object")
		return
	}
	b, err := json.Marshal(m)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not encode settings")
		return
	}
	key := importDialogStoreKey(storedKind)
	if err := h.privateStore.Upsert(r.Context(), key, string(b), mp); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save import settings: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "import dialog settings saved"})
}

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
)

// BackupHandler exposes one-click archive backups (see service.ArchiveBackupService).
//
// Creating a backup writes a file to a filesystem path chosen in the desktop app's native save
// dialog, so it is limited to the archive owner on a local (desktop) deployment talking to the
// server from the same machine. Status/snooze are harmless and available to any session.
type BackupHandler struct {
	svc             *service.ArchiveBackupService
	sessionStore    *keystore.SessionMasterStore
	sensitiveSvc    *service.SensitiveService
	authSvc         *service.AuthService
	deploymentLocal bool
}

// NewBackupHandler creates a BackupHandler.
func NewBackupHandler(svc *service.ArchiveBackupService, sessionStore *keystore.SessionMasterStore, sensitiveSvc *service.SensitiveService, authSvc *service.AuthService, deploymentNature string) *BackupHandler {
	return &BackupHandler{
		svc:             svc,
		sessionStore:    sessionStore,
		sensitiveSvc:    sensitiveSvc,
		authSvc:         authSvc,
		deploymentLocal: strings.EqualFold(strings.TrimSpace(deploymentNature), "local"),
	}
}

// RegisterRoutes mounts the backup routes.
func (h *BackupHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/backup/status", h.Status)
	r.Post("/api/backup", h.Start)
	r.Post("/api/backup/snooze", h.Snooze)
	r.Post("/api/backup/restore-check", h.RestoreCheck)
}

func (h *BackupHandler) available(r *http.Request) bool {
	return h.deploymentLocal && IsLocalhost(r)
}

// Status handles GET /api/backup/status.
func (h *BackupHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hasData := h.svc.ArchiveHasData(ctx)
	st := h.svc.Status(ctx, hasData, time.Now().UTC())
	available := h.available(r)
	if !available {
		st.ReminderDue = false
		st.ArchivePath = ""
		st.LastBackupPath = ""
		st.RunningPath = ""
		if st.Last != nil {
			st.Last.Path = ""
		}
	}
	writeJSON(w, map[string]any{
		"available": available,
		"status":    st,
	})
}

// Start handles POST /api/backup {"path": "<absolute file path>"}.
func (h *BackupHandler) Start(w http.ResponseWriter, r *http.Request) {
	if !h.available(r) {
		writeError(w, http.StatusForbidden, "Backups can only be made from the Digital Museum desktop app.")
		return
	}
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dest, err := h.svc.ValidateDestination(r.Context(), req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := appctx.UserIDFromCtx(r.Context())
	if err := h.svc.Start(uid, dest); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrBackupInProgress) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, map[string]any{"status": "started", "path": dest})
}

// Snooze handles POST /api/backup/snooze {"days": 7}.
func (h *BackupHandler) Snooze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Days <= 0 {
		req.Days = 7
	}
	if err := h.svc.Snooze(r.Context(), req.Days, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

// RestoreCheck handles POST /api/backup/restore-check {"path": "<absolute file path>"}.
//
// The desktop app calls this before it stops the servers to swap the archive file (see
// electron/main.js "restore-archive"), so a wrong, damaged or non-archive file is rejected while
// everything is still running. Refuses while a backup or post-import processing is in progress.
func (h *BackupHandler) RestoreCheck(w http.ResponseWriter, r *http.Request) {
	if !h.available(r) {
		writeError(w, http.StatusForbidden, "Restoring is only possible from the Digital Museum desktop app.")
		return
	}
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if h.svc.Busy() {
		writeError(w, http.StatusConflict, "A backup is being written right now. Try again when it has finished.")
		return
	}
	if postImportBusy(appctx.UserIDFromCtx(r.Context())) {
		writeError(w, http.StatusConflict, "Digital Museum is still preparing recently imported data. Wait for it to finish (or stop it) and try again.")
		return
	}
	cand, err := h.svc.InspectRestoreCandidate(r.Context(), req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "candidate": cand})
}

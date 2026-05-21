package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ProfileHandler serves archive profile endpoints.
// Public:       GET  /profiles           (login page — enabled profiles only)
// Localhost:    POST /api/profiles        (Electron creates a profile)
//
//	GET  /api/profiles/{id}/dbpath
//
// Admin-auth:   GET  /admin/profiles
//
//	PATCH /admin/profiles/{id}
//	DELETE /admin/profiles/{id}
type ProfileHandler struct {
	repo           *repository.ProfileRepo
	requireAdminFn func(http.ResponseWriter, *http.Request) bool
	provision      *service.ArchiveProvisionService
}

// NewProfileHandler creates a ProfileHandler.
// requireAdminFn may be nil when there is no main DB (minimal router mode).
// provision may be nil when archive creation is unavailable (minimal router).
func NewProfileHandler(repo *repository.ProfileRepo, requireAdminFn func(http.ResponseWriter, *http.Request) bool, provision *service.ArchiveProvisionService) *ProfileHandler {
	return &ProfileHandler{repo: repo, requireAdminFn: requireAdminFn, provision: provision}
}

// RegisterRoutes mounts all profile endpoints.
// Admin routes are only registered when requireAdminFn is non-nil (main DB available).
func (h *ProfileHandler) RegisterRoutes(r chi.Router) {
	r.Get("/profiles", h.ListEnabled)
	r.Post("/api/profiles", h.Create)
	r.Get("/api/profiles/{id}/dbpath", h.GetDBPath)
	r.Patch("/api/profiles/{id}", h.PatchProfile)
	if h.requireAdminFn != nil {
		r.Get("/admin/profiles", h.AdminList)
		r.Post("/admin/profiles", h.AdminCreate)
		r.Patch("/admin/profiles/{id}", h.AdminPatch)
		r.Delete("/admin/profiles/{id}", h.AdminDelete)
	}
}

// ListEnabled serves GET /profiles — public, returns enabled profiles only.
// Returns {id, name, username} — no db_path exposed publicly.
func (h *ProfileHandler) ListEnabled(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.repo.ListEnabled(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}
	type publicProfile struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Username  *string `json:"username,omitempty"`
		IsDefault bool    `json:"is_default"`
	}
	out := make([]publicProfile, len(profiles))
	for i, p := range profiles {
		out[i] = publicProfile{ID: p.ID, Name: p.Name, Username: p.Username, IsDefault: p.IsDefault}
	}
	writeJSON(w, out)
}

// Create serves POST /api/profiles — localhost-only; called by Electron.
func (h *ProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !IsLocalhost(r) {
		writeError(w, http.StatusForbidden, "localhost only")
		return
	}
	var body struct {
		Name   string `json:"name"`
		DBPath string `json:"db_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name == "" || body.DBPath == "" {
		writeError(w, http.StatusBadRequest, "name and db_path are required")
		return
	}
	profile, err := h.repo.Create(r.Context(), body.Name, body.DBPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, profile)
}

// GetDBPath serves GET /api/profiles/{id}/dbpath — localhost-only.
func (h *ProfileHandler) GetDBPath(w http.ResponseWriter, r *http.Request) {
	if !IsLocalhost(r) {
		writeError(w, http.StatusForbidden, "localhost only")
		return
	}
	id := chi.URLParam(r, "id")
	profile, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch profile")
		return
	}
	if profile == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, map[string]string{"db_path": profile.DBPath})
}

// PatchProfile serves PATCH /api/profiles/{id} — localhost-only; Electron updates username/name.
func (h *ProfileHandler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	if !IsLocalhost(r) {
		writeError(w, http.StatusForbidden, "localhost only")
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Name      *string `json:"name"`
		Username  *string `json:"username"`
		IsDefault *bool   `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name != nil {
		if err := h.repo.UpdateName(r.Context(), id, *body.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update name")
			return
		}
	}
	if body.Username != nil {
		if err := h.repo.UpdateUsername(r.Context(), id, *body.Username); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update username")
			return
		}
	}
	if body.IsDefault != nil {
		if *body.IsDefault {
			if err := h.repo.SetStartupDefault(r.Context(), id); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		} else {
			if err := h.repo.ClearStartupDefaultForID(r.Context(), id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to clear startup default")
				return
			}
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// AdminList serves GET /admin/profiles — admin-auth, all profiles.
func (h *ProfileHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminFn(w, r) {
		return
	}
	profiles, err := h.repo.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}
	writeJSON(w, map[string]any{"profiles": profiles})
}

// AdminCreate serves POST /admin/profiles — admin-auth; creates a new archive SQLite file,
// registers the first owner, and adds a billing profile row.
func (h *ProfileHandler) AdminCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminFn(w, r) {
		return
	}
	if h.provision == nil {
		writeError(w, http.StatusServiceUnavailable, "archive provisioning is not configured")
		return
	}
	var body struct {
		DisplayName     string `json:"display_name"`
		FamilyName      string `json:"family_name"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		PasswordConfirm string `json:"password_confirm"`
		DBPath          string `json:"db_path"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	displayName := strings.TrimSpace(body.DisplayName)
	familyName := strings.TrimSpace(body.FamilyName)
	username := strings.TrimSpace(body.Username)
	dbPath := strings.TrimSpace(body.DBPath)
	if displayName == "" || familyName == "" || username == "" || body.Password == "" || dbPath == "" {
		writeError(w, http.StatusBadRequest, "display_name, family_name, username, password, and db_path are required")
		return
	}
	if body.Password != body.PasswordConfirm {
		writeError(w, http.StatusBadRequest, "passwords do not match")
		return
	}
	abs := filepath.Clean(dbPath)
	if a, err := filepath.Abs(abs); err == nil {
		abs = a
	}
	archiveName := strings.TrimSpace(displayName + " " + familyName)
	if archiveName == "" {
		archiveName = displayName
	}

	if err := h.provision.CreateArchiveWithFirstUser(r.Context(), abs, displayName, familyName, username, body.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	profile, err := h.repo.Create(r.Context(), archiveName, abs)
	if err != nil {
		// UNIQUE(db_path) or rare races: archive file is already provisioned; reuse billing row
		// instead of deleting the new database and leaving the UI with a misleading generic error.
		existing, gerr := h.repo.GetByDBPath(r.Context(), abs)
		if gerr == nil && existing != nil {
			profile = existing
			if uerr := h.repo.UpdateName(r.Context(), profile.ID, archiveName); uerr != nil {
				writeError(w, http.StatusInternalServerError, "failed to update archive profile name: "+uerr.Error())
				return
			}
		} else {
			_ = os.Remove(abs)
			writeError(w, http.StatusBadRequest, "failed to register archive in billing: "+err.Error())
			return
		}
	}
	if err := h.repo.UpdateUsername(r.Context(), profile.ID, strings.ToLower(username)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set archive username")
		return
	}
	if body.Enabled != nil && !*body.Enabled {
		if err := h.repo.SetEnabled(r.Context(), profile.ID, false); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update enabled")
			return
		}
	}
	_ = h.repo.PromoteSoleEnabledArchiveToDefault(r.Context())

	created, err := h.repo.GetByID(r.Context(), profile.ID)
	if err != nil || created == nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch created profile")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(created); err != nil {
		_ = err
	}
}

// AdminPatch serves PATCH /admin/profiles/{id} — admin-auth, enable/disable.
func (h *ProfileHandler) AdminPatch(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminFn(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Enabled   *bool   `json:"enabled"`
		Name      *string `json:"name"`
		Username  *string `json:"username"`
		IsDefault *bool   `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Enabled != nil {
		if err := h.repo.SetEnabled(r.Context(), id, *body.Enabled); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update enabled")
			return
		}
	}
	if body.Name != nil {
		if err := h.repo.UpdateName(r.Context(), id, *body.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update name")
			return
		}
	}
	if body.Username != nil {
		if err := h.repo.UpdateUsername(r.Context(), id, *body.Username); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update username")
			return
		}
	}
	if body.IsDefault != nil {
		if *body.IsDefault {
			if err := h.repo.SetStartupDefault(r.Context(), id); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		} else {
			if err := h.repo.ClearStartupDefaultForID(r.Context(), id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to clear startup default")
				return
			}
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// AdminDelete serves DELETE /admin/profiles/{id} — admin-auth.
// Query parameter delete_file=true also removes the SQLite file from disk.
// Returns JSON {ok, file_deleted, file_warning?} so the UI can surface partial
// success when the row was removed but the on-disk file could not be deleted
// (e.g. it is currently held open by the running server, or already missing).
func (h *ProfileHandler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminFn(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	deleteFile := r.URL.Query().Get("delete_file") == "true"

	// When the file is to be removed, fetch the path before deleting the row.
	var dbPath string
	if deleteFile {
		profile, err := h.repo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch profile")
			return
		}
		if profile == nil {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		dbPath = profile.DBPath
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete profile")
		return
	}

	resp := map[string]any{"ok": true, "file_deleted": false}
	if deleteFile && dbPath != "" {
		if err := os.Remove(dbPath); err != nil {
			if os.IsNotExist(err) {
				resp["file_warning"] = "Database file was already missing on disk."
			} else {
				resp["file_warning"] = "Profile removed, but the database file could not be deleted: " + err.Error()
			}
		} else {
			resp["file_deleted"] = true
		}
	}
	writeJSON(w, resp)
}

// ResolvedMainSQLitePath serves GET /api/resolved-main-sqlite-path — localhost only.
// Returns the effective main archive path after server startup (billing default only).
func ResolvedMainSQLitePath(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !IsLocalhost(r) {
			writeError(w, http.StatusForbidden, "localhost only")
			return
		}
		writeJSON(w, map[string]string{"sqlite_path": strings.TrimSpace(cfg.DB.SQLitePath)})
	}
}

// IsLocalhost returns true when the request originates from the local machine.
func IsLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1"
}

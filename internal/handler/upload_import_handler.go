package handler

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	facebookimport "github.com/daveontour/aimuseum/internal/import/facebook"
	facebookalbumsimport "github.com/daveontour/aimuseum/internal/import/facebookalbums"
	facebookplacesimport "github.com/daveontour/aimuseum/internal/import/facebookplaces"
	facebookpostsimport "github.com/daveontour/aimuseum/internal/import/facebookposts"
	imessageimport "github.com/daveontour/aimuseum/internal/import/imessage"
	instagramimport "github.com/daveontour/aimuseum/internal/import/instagram"
	whatsappimport "github.com/daveontour/aimuseum/internal/import/whatsapp"
	appimporter "github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/importstorage"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ── ZIP import job singleton ──────────────────────────────────────────────────

var uploadJob = appimporter.NewImportJob("ZIP import", map[string]any{
	"status":        "idle",
	"status_line":   nil,
	"error_message": nil,
	"import_type":   nil,
	"job_id":        nil,
})

// ── UploadImportHandler ───────────────────────────────────────────────────────

// UploadImportHandler handles local-path import endpoints.
type UploadImportHandler struct {
	pool              *sql.DB
	subjectConfigRepo *repository.SubjectConfigRepo
	sessionStore      *keystore.SessionMasterStore
	sensitiveSvc      *service.SensitiveService
	authSvc           *service.AuthService
	uploadCfg         config.UploadConfig
}

// NewUploadImportHandler creates an UploadImportHandler.
func NewUploadImportHandler(pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, sessionStore *keystore.SessionMasterStore, sensitiveSvc *service.SensitiveService, authSvc *service.AuthService, uploadCfg config.UploadConfig) *UploadImportHandler {
	return &UploadImportHandler{pool: pool, subjectConfigRepo: subjectRepo, sessionStore: sessionStore, sensitiveSvc: sensitiveSvc, authSvc: authSvc, uploadCfg: uploadCfg}
}

// RegisterRoutes mounts import routes.
func (h *UploadImportHandler) RegisterRoutes(r chi.Router) error {
	r.Post("/import/from-path", h.ImportFromPath)
	r.Get("/import/upload/stream", h.ImportStream)
	r.Post("/import/upload/cancel", h.ImportCancel)
	r.Get("/import/upload/status", h.ImportStatus)
	r.Get("/import/jobs", h.Jobs)
	r.Get("/api/upload-config", h.UploadConfigJSON)
	return nil
}

// ── POST /import/from-path ────────────────────────────────────────────────────

// ImportFromPath starts a background import from a local filesystem path.
// file_path may be a .zip archive or an already-extracted directory.
// When a directory is given, the files are read directly — no extraction step.
func (h *UploadImportHandler) ImportFromPath(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := uploadJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	var req struct {
		FilePath string `json:"file_path"`
		Type     string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	req.FilePath = strings.TrimSpace(req.FilePath)
	validTypes := map[string]bool{
		"facebook":  true,
		"instagram": true,
		"whatsapp":  true,
		"imessage":  true,
	}
	if !validTypes[req.Type] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid import type %q; must be one of: facebook, instagram, whatsapp, imessage", req.Type))
		return
	}
	if req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "file_path is required")
		return
	}

	uid := appctx.UserIDFromCtx(r.Context())
	jobID := randomJobID()
	uidStr := fmt.Sprintf("%d", uid)
	if uid == 0 {
		uidStr = "0"
	}
	tmpDir := filepath.Join("tmp", "imports", uidStr, jobID)
	uploadJob.Start()
	setUploadJobStage("in_progress", fmt.Sprintf("Queued %s import from %s", req.Type, req.FilePath), req.Type, nil, jobID)
	go h.preparePathImportAndStart(uid, req.Type, req.FilePath, tmpDir, jobID)
	writeJSON(w, map[string]any{
		"message": fmt.Sprintf("%s import queued from %s", req.Type, req.FilePath),
		"job_id":  jobID,
	})
}

// ── GET /import/upload/stream ─────────────────────────────────────────────────

// ImportStream serves SSE progress for the ZIP import job.
func (h *UploadImportHandler) ImportStream(w http.ResponseWriter, r *http.Request) {
	uploadJob.ServeSSE(w, r)
}

// ── GET /import/upload/status ─────────────────────────────────────────────────

// ImportStatus returns the current JSON status of the ZIP import job.
func (h *UploadImportHandler) ImportStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, uploadJob.Status())
}

// ImportCancel handles POST /import/upload/cancel.
func (h *UploadImportHandler) ImportCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, uploadJob.Cancel())
}

// startZipImportBackground starts the shared ZIP import goroutine. tmpDir is the job temp
// root (containing extracted/); it is removed when the import finishes.
// If jobAlreadyStarted is true (TUS path after signalUploadZipTUSPreExtract), Start() is skipped.
func (h *UploadImportHandler) startZipImportBackground(uid int64, importType, tmpDir, importRoot string, jobAlreadyStarted bool) {
	if !jobAlreadyStarted {
		uploadJob.Start()
	}
	setUploadJobStage("in_progress", fmt.Sprintf("Starting %s import...", importType), importType, nil, nil)
	uploadJob.Broadcast("status", map[string]any{"status_line": fmt.Sprintf("Starting %s import...", importType)})

	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	go func() {
		defer os.RemoveAll(tmpDir)
		switch importType {
		case "whatsapp":
			runUploadWhatsApp(ctx, h.pool, h.subjectConfigRepo, uploadJob, importRoot)
		case "imessage":
			runUploadIMessage(ctx, h.pool, h.subjectConfigRepo, uploadJob, importRoot)
		case "instagram":
			runUploadInstagram(ctx, h.pool, h.subjectConfigRepo, uploadJob, importRoot)
		case "facebook":
			runUploadFacebook(ctx, h.pool, h.subjectConfigRepo, uploadJob, importRoot)
		}
	}()
}

func setUploadJobStage(status, statusLine, importType string, err error, jobID any) {
	updates := map[string]any{
		"status":        status,
		"status_line":   statusLine,
		"error_message": nil,
		"import_type":   importType,
	}
	if err != nil {
		updates["error_message"] = err.Error()
	}
	if jobID != nil {
		updates["job_id"] = jobID
	}
	uploadJob.UpdateState(updates)
	uploadJob.Broadcast("progress", uploadJob.GetState())
}

func (h *UploadImportHandler) preparePathImportAndStart(uid int64, importType, filePath, tmpDir, jobID string) {
	handlerOwnsCleanup := true
	defer func() {
		if handlerOwnsCleanup {
			os.RemoveAll(tmpDir)
		}
	}()
	defer func() {
		if handlerOwnsCleanup {
			uploadJob.Finish()
		}
	}()

	setUploadJobStage("in_progress", "Validating source path...", importType, nil, jobID)
	info, err := os.Stat(filePath)
	if err != nil {
		setUploadJobStage("error", "Path not found: "+filePath, importType, err, jobID)
		uploadJob.Broadcast("error", uploadJob.GetState())
		return
	}

	if uploadJob.IsCancelled() {
		setUploadJobStage("cancelled", "Import cancelled before file handling began.", importType, nil, jobID)
		uploadJob.Broadcast("cancelled", uploadJob.GetState())
		return
	}

	if info.IsDir() {
		setUploadJobStage("in_progress", "Source is a directory; preparing import root...", importType, nil, jobID)
		// tmpDir is created so startZipImportBackground can clean it up safely.
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			setUploadJobStage("error", "Failed to create temp directory", importType, err, jobID)
			uploadJob.Broadcast("error", uploadJob.GetState())
			return
		}
		setUploadJobStage("in_progress", "Directory ready; resolving import root...", importType, nil, jobID)
		importRoot := resolveImportRoot(filePath)
		handlerOwnsCleanup = false
		h.startZipImportBackground(uid, importType, tmpDir, importRoot, true)
		return
	}

	if !strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		err := fmt.Errorf("path must be a .zip file or a directory")
		setUploadJobStage("error", err.Error(), importType, err, jobID)
		uploadJob.Broadcast("error", uploadJob.GetState())
		return
	}

	setUploadJobStage("in_progress", "ZIP file detected; preparing extraction directory...", importType, nil, jobID)
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		setUploadJobStage("error", "Failed to create temp directory", importType, err, jobID)
		uploadJob.Broadcast("error", uploadJob.GetState())
		return
	}

	if uploadJob.IsCancelled() {
		setUploadJobStage("cancelled", "Import cancelled before ZIP extraction.", importType, nil, jobID)
		uploadJob.Broadcast("cancelled", uploadJob.GetState())
		return
	}

	setUploadJobStage("in_progress", "Extracting ZIP archive...", importType, nil, jobID)
	if err := extractZip(filePath, extractDir); err != nil {
		setUploadJobStage("error", "Failed to extract ZIP archive", importType, err, jobID)
		uploadJob.Broadcast("error", uploadJob.GetState())
		return
	}

	if uploadJob.IsCancelled() {
		setUploadJobStage("cancelled", "Import cancelled after ZIP extraction.", importType, nil, jobID)
		uploadJob.Broadcast("cancelled", uploadJob.GetState())
		return
	}

	setUploadJobStage("in_progress", "ZIP extracted; resolving import root...", importType, nil, jobID)
	importRoot := resolveImportRoot(extractDir)
	handlerOwnsCleanup = false
	h.startZipImportBackground(uid, importType, tmpDir, importRoot, true)
}

// UploadConfigJSON exposes tus chunk size and max upload GiB for the SPA.
func (h *UploadImportHandler) UploadConfigJSON(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"chunkSizeMB": h.uploadCfg.TUSChunkSizeMB,
		"maxUploadGB": h.uploadCfg.MaxUploadGB,
	})
}

// ── GET /import/jobs ──────────────────────────────────────────────────────────

// Jobs returns a JSON map of all import job statuses.
func (h *UploadImportHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	jobs := map[string]any{
		"upload":           uploadJob.Status(),
		"filesystem":       filesystemJob.Status(),
		"whatsapp":         whatsappJob.Status(),
		"imessage":         imessageJob.Status(),
		"instagram":        instagramJob.Status(),
		"facebook_all":     facebookAllJob.Status(),
		"thumbnails":       thumbnailsJob.Status(),
		"contacts_extract": contactsExtractJob.Status(),
		"reference_import": referenceImportJob.Status(),
		"image_regions_recalc": imageRegionsRecalcJob.Status(),
	}
	writeJSON(w, jobs)
}

// ── Upload run functions ───────────────────────────────────────────────────────

func runUploadWhatsApp(ctx context.Context, pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *appimporter.ImportJob, directoryPath string) {
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats whatsappimport.ImportStats, justCompleted string) {
		// statusLine := fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
		// 	stats.ConversationsProcessed, stats.TotalConversations, justCompleted,
		// 	stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
		// 	stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
		statusLine := fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found",
			stats.ConversationsProcessed, stats.TotalConversations, justCompleted,
			stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
			stats.AttachmentsFound)
		job.UpdateState(map[string]any{
			"status_line": statusLine,
		})
		job.Broadcast("progress", job.GetState())
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := whatsappimport.ImportWhatsAppFromDirectory(ctx, storage, directoryPath, progressCallback, cancelledCheck)

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}
	if err != nil {
		msg := fmt.Sprintf("import error: %s", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	statusLine := fmt.Sprintf("Completed: %d conversations, %d messages (%d created, %d updated), %d attachments found, %d missing",
		stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
		stats.AttachmentsFound, stats.AttachmentsMissing)
	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": statusLine,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func runUploadIMessage(ctx context.Context, pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *appimporter.ImportJob, directoryPath string) {
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats imessageimport.ImportStats) {
		statusLine := ""
		if stats.TotalConversations > 0 {
			// statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
			// 	stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
			// 	stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
			// 	stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found",
				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
				stats.AttachmentsFound)
		}
		job.UpdateState(map[string]any{
			"status_line": statusLine,
		})
		job.Broadcast("progress", job.GetState())
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := imessageimport.ImportIMessagesFromDirectory(ctx, storage, directoryPath, progressCallback, cancelledCheck)

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}
	if err != nil {
		msg := fmt.Sprintf("import error: %s", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	statusLine := fmt.Sprintf("Completed: %d conversations, %d messages (%d created, %d updated), %d attachments found, %d missing",
		stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
		stats.AttachmentsFound, stats.AttachmentsMissing)
	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": statusLine,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func runUploadInstagram(ctx context.Context, pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *appimporter.ImportJob, directoryPath string) {
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats instagramimport.ImportStats) {
		statusLine := ""
		if stats.TotalConversations > 0 {
			// statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
			// 	stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
			// 	stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
			// 	stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found",
				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
				stats.AttachmentsFound)
		}
		job.UpdateState(map[string]any{
			"status_line": statusLine,
		})
		job.Broadcast("progress", job.GetState())
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := instagramimport.ImportInstagramFromDirectory(ctx, storage, directoryPath, progressCallback, cancelledCheck, "", "")

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}
	if err != nil {
		msg := fmt.Sprintf("import error: %s", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	statusLine := fmt.Sprintf("Completed: %d conversations, %d messages (%d created, %d updated)",
		stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated)
	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": statusLine,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func runUploadFacebook(ctx context.Context, pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *appimporter.ImportJob, directoryPath string) {
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)
	exportRoot := directoryPath
	cancelledCheck := func() bool { return job.IsCancelled() }

	// Messenger
	job.UpdateState(map[string]any{"status_line": "Running Facebook Messenger import..."})
	job.Broadcast("progress", job.GetState())

	messengerProgress := func(stats facebookimport.ImportStats) {
		job.UpdateState(map[string]any{
			"status_line": fmt.Sprintf("Messenger: %d conversations, %d messages (%d created, %d updated)",
				stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated),
		})
		job.Broadcast("progress", job.GetState())
	}
	messengerStats, messengerErr := facebookimport.ImportFacebookFromDirectory(
		ctx, storage, directoryPath, messengerProgress, cancelledCheck, exportRoot, "",
	)

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	// Albums
	job.UpdateState(map[string]any{"status_line": "Running Facebook Albums import..."})
	job.Broadcast("progress", job.GetState())

	albumsProgress := func(stats facebookalbumsimport.ImportStats) {
		job.UpdateState(map[string]any{
			"status_line": fmt.Sprintf("Albums: %d processed, %d imported, %d images imported",
				stats.AlbumsProcessed, stats.AlbumsImported, stats.ImagesImported),
		})
		job.Broadcast("progress", job.GetState())
	}
	albumsStats, albumsErr := facebookalbumsimport.ImportFacebookAlbumsFromDirectory(
		ctx, pool, directoryPath, albumsProgress, cancelledCheck, exportRoot,
	)

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	// Places
	job.UpdateState(map[string]any{"status_line": "Running Facebook Places import..."})
	job.Broadcast("progress", job.GetState())

	placesProgress := func(stats facebookplacesimport.ImportStats) {
		job.UpdateState(map[string]any{
			"status_line": fmt.Sprintf("Places: %d imported (%d created, %d updated)",
				stats.PlacesImported, stats.PlacesCreated, stats.PlacesUpdated),
		})
		job.Broadcast("progress", job.GetState())
	}
	placesStats, placesErr := facebookplacesimport.ImportFacebookPlacesFromDirectory(
		ctx, pool, directoryPath, placesProgress, cancelledCheck,
	)

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	// Posts
	job.UpdateState(map[string]any{"status_line": "Running Facebook Posts import..."})
	job.Broadcast("progress", job.GetState())

	postsProgress := func(stats facebookpostsimport.ImportStats) {
		job.UpdateState(map[string]any{
			"status_line": fmt.Sprintf("Posts: %d processed, %d imported, %d updated",
				stats.PostsProcessed, stats.PostsImported, stats.PostsUpdated),
		})
		job.Broadcast("progress", job.GetState())
	}
	postsStats, postsErr := facebookpostsimport.ImportFacebookPostsFromPath(
		ctx, pool, directoryPath, exportRoot, postsProgress, cancelledCheck,
	)

	// Build summary
	var parts []string
	if messengerStats != nil {
		parts = append(parts, fmt.Sprintf("Messenger: %d conversations, %d messages",
			messengerStats.ConversationsProcessed, messengerStats.MessagesImported))
	}
	if albumsStats != nil {
		parts = append(parts, fmt.Sprintf("Albums: %d imported, %d images",
			albumsStats.AlbumsImported, albumsStats.ImagesImported))
	}
	if placesStats != nil {
		parts = append(parts, fmt.Sprintf("Places: %d imported",
			placesStats.PlacesImported))
	}
	if postsStats != nil {
		parts = append(parts, fmt.Sprintf("Posts: %d imported",
			postsStats.PostsImported))
	}

	anyErr := messengerErr != nil || albumsErr != nil || placesErr != nil || postsErr != nil
	if anyErr {
		var errMsgs []string
		if messengerErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("Messenger: %v", messengerErr))
		}
		if albumsErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("Albums: %v", albumsErr))
		}
		if placesErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("Places: %v", placesErr))
		}
		if postsErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("Posts: %v", postsErr))
		}
		errSummary := strings.Join(errMsgs, "; ")
		job.UpdateState(map[string]any{
			"status":        "completed",
			"status_line":   "Import completed with errors: " + errSummary,
			"error_message": errSummary,
		})
	} else {
		statusLine := "Completed: " + strings.Join(parts, " | ")
		job.UpdateState(map[string]any{
			"status":      "completed",
			"status_line": statusLine,
		})
	}

	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// randomJobID generates a random hex job ID.
func randomJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// resolveImportRoot returns the actual data root inside an extracted ZIP directory.
// Most platform export ZIPs contain a single top-level folder
// (e.g. "facebook-export-2024-01-01/") wrapping all data. When extractDir holds
// exactly one sub-directory and no loose files, that sub-directory is returned as
// the import root. Otherwise extractDir itself is returned unchanged.
func resolveImportRoot(extractDir string) string {
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return extractDir
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		} else {
			// A loose file at the root means this IS the root
			return extractDir
		}
	}
	if len(dirs) == 1 {
		return filepath.Join(extractDir, dirs[0])
	}
	return extractDir
}

// extractZip extracts a ZIP archive to destDir, rejecting path-traversal entries.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") {
			continue
		}
		destPath := filepath.Join(destDir, name)
		// Use filepath.Rel for OS-independent containment check (avoids the
		// mixed-separator false-negative that breaks the slash-based check on Windows).
		rel, err := filepath.Rel(destDir, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(destPath, 0755)
			continue
		}
		if err := extractZipEntry(f, destPath); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

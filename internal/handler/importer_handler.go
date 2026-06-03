package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"

	"strings"
	"sync"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/appctx"
	contactsimport "github.com/daveontour/aimuseum/internal/import/contacts"
	facebookimport "github.com/daveontour/aimuseum/internal/import/facebook"
	facebookalbumsimport "github.com/daveontour/aimuseum/internal/import/facebookalbums"
	facebookallimport "github.com/daveontour/aimuseum/internal/import/facebookall"
	facebookplacesimport "github.com/daveontour/aimuseum/internal/import/facebookplaces"
	facebookpostsimport "github.com/daveontour/aimuseum/internal/import/facebookposts"
	filesystemimport "github.com/daveontour/aimuseum/internal/import/filesystem"
	imessageimport "github.com/daveontour/aimuseum/internal/import/imessage"
	instagramimport "github.com/daveontour/aimuseum/internal/import/instagram"
	thumbnailsimport "github.com/daveontour/aimuseum/internal/import/thumbnails"
	whatsappimport "github.com/daveontour/aimuseum/internal/import/whatsapp"
	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/importstorage"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/daveontour/aimuseum/internal/sqlutil"
	"github.com/go-chi/chi/v5"
)

// ── Job singletons ────────────────────────────────────────────────────────────

var (
	whatsappJob = importer.NewImportJob("WhatsApp import", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "attachments_found": 0, "attachments_missing": 0,
		"missing_attachment_filenames": []string{}, "errors": 0,
	})

	imessageJob = importer.NewImportJob("iMessage import", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "attachments_found": 0, "attachments_missing": 0,
		"missing_attachment_filenames": []string{}, "errors": 0,
	})

	instagramJob = importer.NewImportJob("Instagram import", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "errors": 0,
	})

	emailProcessJob = importer.NewImportJob("Email (Gmail) import", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"emails_processed": 0, "current_label": nil, "total_labels": 0,
	})

	// facebookMessengerJob = importer.NewImportJob("Facebook Messenger import", map[string]any{
	// 	"status": "idle", "status_line": nil, "error_message": nil,
	// 	"conversations": 0, "messages_imported": 0, "messages_created": 0,
	// 	"messages_updated": 0, "attachments_found": 0, "attachments_missing": 0,
	// 	"missing_attachment_filenames": []string{}, "errors": 0,
	// })

	filesystemJob = importer.NewImportJob("Filesystem images import", map[string]any{
		"status": "idle", "status_line": nil, "current_file": nil,
		"files_processed": 0, "total_files": 0, "images_imported": 0,
		"images_referenced": 0, "images_updated": 0,
		"errors": 0, "error_messages": []string{}, "log_lines": []string{},
	})

	// facebookAlbumsJob = importer.NewImportJob("Facebook Albums import", map[string]any{
	// 	"status": "idle", "status_line": nil, "current_album": nil,
	// 	"albums_processed": 0, "total_albums": 0, "albums_imported": 0,
	// 	"images_imported": 0, "images_found": 0, "images_missing": 0,
	// 	"missing_image_filenames": []string{},
	// 	"errors":                  0, "error_message": nil,
	// })

	// facebookPostsJob = importer.NewImportJob("Facebook Posts import", map[string]any{
	// 	"status": "idle", "status_line": nil, "error_message": nil,
	// 	"posts_processed": 0, "posts_imported": 0, "posts_updated": 0,
	// 	"with_media": 0, "images_imported": 0, "images_found": 0,
	// 	"images_missing": 0, "errors": 0,
	// })

	// facebookPlacesJob = importer.NewImportJob("Facebook Places import", map[string]any{
	// 	"status": "idle", "status_line": nil, "error_message": nil,
	// 	"places_imported": 0, "places_created": 0, "places_updated": 0,
	// })

	facebookAllJob = importer.NewImportJob("Facebook All import", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "att_found": 0, "att_missing": 0, "messenger_errors": 0,
		"albums_processed": 0, "albums_imported": 0, "album_images_imported": 0,
		"album_images_found": 0, "album_images_missing": 0, "albums_errors": 0,
		"places_imported": 0, "places_created": 0, "places_updated": 0,
		"posts_processed": 0, "posts_imported": 0, "posts_updated": 0,
		"with_media": 0, "images_imported": 0, "images_found": 0,
		"images_missing": 0, "posts_errors": 0,
	})

	thumbnailsJob = importer.NewImportJob("Thumbnail processing", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"phase": nil, "phase1_scanned": 0, "phase1_updated": 0,
		"phase2_scanned": 0, "phase2_total": 0,
		"phase2_processed": 0, "phase2_errors": 0,
	})

	contactsExtractJob = importer.NewImportJob("Contacts extract", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"contacts_processed": 0, "contacts_merged": 0, "contacts_created": 0,
	})

	referenceImportJob = importer.NewImportJob("Reference import", map[string]any{
		"status": "idle", "status_line": nil, "total": 0, "processed": 0,
		"imported": 0, "skipped": 0, "errors": 0, "error_message": nil, "error_messages": []string{},
	})

	imageRegionsRecalcJob = importer.NewImportJob("Image region recalculation", map[string]any{
		"status": "idle", "status_line": nil, "total": 0, "processed": 0,
		"updated": 0, "error_message": nil,
	})

	mediaClassificationTagQAJob = importer.NewImportJob("Media classification tag QA", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "reflagged": 0, "errors": 0,
	})

	mediaGPSDuplicateSpreadJob = importer.NewImportJob("Media GPS duplicate spread", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "updated": 0, "errors": 0,
	})

	imageExportJob = importer.NewImportJob("Image export", map[string]any{
		"status": "idle", "status_line": nil, "total": 0, "processed": 0,
		"exported": 0, "skipped": 0, "errors": 0, "error_message": nil, "error_messages": []string{},
	})

	imageMetadataJSONExportJob = importer.NewImportJob("Image metadata JSON export", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "exported": 0, "errors": 0,
	})

	emailEmbeddingBackfillJob = importer.NewImportJob("Email embedding backfill", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
	})

	messageEmbeddingBackfillJob = importer.NewImportJob("Message embedding backfill", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
	})

	messageContextEmbeddingBackfillJob = importer.NewImportJob("Message context embedding backfill", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
	})
)

// ── ImporterHandler ───────────────────────────────────────────────────────────

// ImporterHandler handles all import job HTTP endpoints.
type ImporterHandler struct {
	excludePatterns   []string
	imageRepo         *repository.ImageRepo
	pool              *sql.DB
	subjectConfigRepo *repository.SubjectConfigRepo
	sessionStore      *keystore.SessionMasterStore
	sensitiveSvc      *service.SensitiveService
	authSvc           *service.AuthService
	embeddingSvc      *service.EmbeddingService
}

// ImporterHandlerDeps holds dependencies for NewImporterHandler.
type ImporterHandlerDeps struct {
	ExcludePatterns   []string
	ImageRepo         *repository.ImageRepo
	Pool              *sql.DB
	SubjectConfigRepo *repository.SubjectConfigRepo
	SessionStore      *keystore.SessionMasterStore
	SensitiveSvc      *service.SensitiveService
	AuthSvc           *service.AuthService
	EmbeddingSvc      *service.EmbeddingService
}

// NewImporterHandler creates an ImporterHandler.
func NewImporterHandler(deps ImporterHandlerDeps) *ImporterHandler {
	return &ImporterHandler{
		excludePatterns:   deps.ExcludePatterns,
		imageRepo:         deps.ImageRepo,
		pool:              deps.Pool,
		subjectConfigRepo: deps.SubjectConfigRepo,
		sessionStore:      deps.SessionStore,
		sensitiveSvc:      deps.SensitiveSvc,
		authSvc:           deps.AuthSvc,
		embeddingSvc:      deps.EmbeddingSvc,
	}
}

// RegisterRoutes mounts all import job routes.
func (h *ImporterHandler) RegisterRoutes(r chi.Router) {
	// Filesystem
	r.Post("/images/import", h.FilesystemStart)
	r.Get("/images/import/stream", h.FilesystemStream)
	r.Post("/images/import/cancel", h.FilesystemCancel)
	r.Get("/images/import/status", h.FilesystemStatus)

	// Thumbnails
	r.Post("/images/process-thumbnails", h.ThumbnailsStart)
	r.Post("/images/process-thumbnails/async", h.ThumbnailsStartAsync)
	r.Get("/images/process-thumbnails/stream", h.ThumbnailsStream)
	r.Post("/images/process-thumbnails/cancel", h.ThumbnailsCancel)
	r.Get("/images/process-thumbnails/status", h.ThumbnailsStatus)

	// Facebook Albums
	// r.Post("/facebook/albums/import", h.FacebookAlbumsStart)
	// r.Get("/facebook/albums/import/stream", h.FacebookAlbumsStream)
	// r.Post("/facebook/albums/import/cancel", h.FacebookAlbumsCancel)
	// r.Get("/facebook/albums/import/status", h.FacebookAlbumsStatus)

	// // Facebook Posts
	// r.Post("/facebook/posts/import", h.FacebookPostsStart)
	// r.Get("/facebook/posts/import/stream", h.FacebookPostsStream)
	// r.Post("/facebook/posts/import/cancel", h.FacebookPostsCancel)
	// r.Get("/facebook/posts/import/status", h.FacebookPostsStatus)

	// // Facebook Places
	// r.Post("/facebook/import-places", h.FacebookPlacesStart)
	// r.Get("/facebook/import-places/stream", h.FacebookPlacesStream)
	// r.Post("/facebook/import-places/cancel", h.FacebookPlacesCancel)
	// r.Get("/facebook/import-places/status", h.FacebookPlacesStatus)

	// Facebook All
	r.Post("/facebook/all/import", h.FacebookAllStart)
	r.Get("/facebook/all/import/stream", h.FacebookAllStream)
	r.Post("/facebook/all/import/cancel", h.FacebookAllCancel)
	r.Get("/facebook/all/import/status", h.FacebookAllStatus)

	// Contacts extract
	r.Post("/contacts/extract", h.ContactsExtractStart)
	r.Get("/contacts/extract/stream", h.ContactsExtractStream)
	r.Post("/contacts/extract/cancel", h.ContactsExtractCancel)
	r.Get("/contacts/extract/status", h.ContactsExtractStatus)

	// Reference import (import referenced images into database)
	r.Post("/images/import-reference", h.ReferenceImportStart)
	r.Get("/images/import-reference/stream", h.ReferenceImportStream)
	r.Post("/images/import-reference/cancel", h.ReferenceImportCancel)
	r.Get("/images/import-reference/status", h.ReferenceImportStatus)

	// Image region recalculation (GPS → region code)
	r.Post("/images/recalculate-regions", h.ImageRegionsRecalcStart)
	r.Get("/images/recalculate-regions/stream", h.ImageRegionsRecalcStream)
	r.Post("/images/recalculate-regions/cancel", h.ImageRegionsRecalcCancel)
	r.Get("/images/recalculate-regions/status", h.ImageRegionsRecalcStatus)

	// Media classification tag QA (re-flag under-tagged classified images)
	r.Post("/images/classification-tag-qa", h.MediaClassificationTagQAStart)
	r.Get("/images/classification-tag-qa/stream", h.MediaClassificationTagQAStream)
	r.Post("/images/classification-tag-qa/cancel", h.MediaClassificationTagQACancel)
	r.Get("/images/classification-tag-qa/status", h.MediaClassificationTagQAStatus)

	// Spread duplicate GPS coordinates on image media_items
	r.Post("/images/gps-duplicate-spread", h.MediaGPSDuplicateSpreadStart)
	r.Get("/images/gps-duplicate-spread/stream", h.MediaGPSDuplicateSpreadStream)
	r.Post("/images/gps-duplicate-spread/cancel", h.MediaGPSDuplicateSpreadCancel)
	r.Get("/images/gps-duplicate-spread/status", h.MediaGPSDuplicateSpreadStatus)

	// Image export (export images to filesystem)
	r.Post("/images/export", h.ImageExportStart)
	r.Get("/images/export/stream", h.ImageExportStream)
	r.Post("/images/export/cancel", h.ImageExportCancel)
	r.Get("/images/export/status", h.ImageExportStatus)

	// Image metadata JSON export
	r.Post("/images/metadata-json-export", h.ImageMetadataJSONExportStart)
	r.Get("/images/metadata-json-export/stream", h.ImageMetadataJSONExportStream)
	r.Post("/images/metadata-json-export/cancel", h.ImageMetadataJSONExportCancel)
	r.Get("/images/metadata-json-export/status", h.ImageMetadataJSONExportStatus)

	// WhatsApp
	r.Post("/whatsapp/import", h.WhatsAppStart)
	r.Get("/whatsapp/import/stream", h.WhatsAppStream)
	r.Post("/whatsapp/import/cancel", h.WhatsAppCancel)
	r.Get("/whatsapp/import/status", h.WhatsAppStatus)

	// iMessage
	r.Post("/imessages/import", h.IMessageStart)
	r.Get("/imessages/import/stream", h.IMessageStream)
	r.Post("/imessages/import/cancel", h.IMessageCancel)
	r.Get("/imessages/import/status", h.IMessageStatus)

	// Instagram
	r.Post("/instagram/import", h.InstagramStart)
	r.Get("/instagram/import/stream", h.InstagramStream)
	r.Post("/instagram/import/cancel", h.InstagramCancel)
	r.Get("/instagram/import/status", h.InstagramStatus)

	// Facebook Messenger (standalone)
	// r.Post("/facebook/import", h.FacebookMessengerStart)
	// r.Get("/facebook/import/stream", h.FacebookMessengerStream)
	// r.Post("/facebook/import/cancel", h.FacebookMessengerCancel)
	// r.Get("/facebook/import/status", h.FacebookMessengerStatus)

	// Email (Gmail) process — stub: Gmail OAuth is not implemented in Go; use /imap/process
	r.Post("/emails/process", h.EmailProcessStart)
	r.Get("/emails/process/stream", h.EmailProcessStream)
	r.Post("/emails/process/cancel", h.EmailProcessCancel)
	r.Get("/emails/process/status", h.EmailProcessStatus)

	// Email embedding backfill (SQLite: email_embeddings vec0; PG: emails.embedding_vector)
	r.Post("/emails/embeddings/backfill", h.EmailEmbeddingBackfillStart)
	r.Get("/emails/embeddings/backfill/stream", h.EmailEmbeddingBackfillStream)
	r.Post("/emails/embeddings/backfill/cancel", h.EmailEmbeddingBackfillCancel)
	r.Get("/emails/embeddings/backfill/status", h.EmailEmbeddingBackfillStatus)

	// // Message embedding backfill (embed rows where embedding_vector is NULL)
	//superseeded by message context embedding backfill
	// r.Post("/messages/embeddings/backfill", h.MessageEmbeddingBackfillStart)
	// r.Get("/messages/embeddings/backfill/stream", h.MessageEmbeddingBackfillStream)
	// r.Post("/messages/embeddings/backfill/cancel", h.MessageEmbeddingBackfillCancel)
	// r.Get("/messages/embeddings/backfill/status", h.MessageEmbeddingBackfillStatus)

	// Message context embedding backfill (windows of prev/current/next messages into sqlite-vec table)
	r.Post("/messages/context-embeddings/backfill", h.MessageContextEmbeddingBackfillStart)
	r.Get("/messages/context-embeddings/backfill/stream", h.MessageContextEmbeddingBackfillStream)
	r.Post("/messages/context-embeddings/backfill/cancel", h.MessageContextEmbeddingBackfillCancel)
	r.Get("/messages/context-embeddings/backfill/status", h.MessageContextEmbeddingBackfillStatus)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// runThumbnailsAfterImportIfIdle starts thumbnail processing if the thumbnails job is idle.
// Called after image-loading imports (filesystem, etc.) and email imports (IMAP/Gmail) complete successfully.
func runThumbnailsAfterImportIfIdle(pool *sql.DB) {
	if pool == nil {
		return
	}
	if err := thumbnailsJob.AssertNotRunning(); err != nil {
		return // already running, skip
	}
	thumbnailsJob.Start()
	thumbnailsJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting thumbnail processing (auto after import)...",
		"phase": nil, "phase1_scanned": 0, "phase1_updated": 0,
		"phase2_scanned": 0, "phase2_total": 0, "phase2_processed": 0, "phase2_errors": 0,
	})
	thumbnailsJob.Broadcast("status", map[string]any{"status_line": "Starting thumbnail processing (auto after import)..."})
	go runThumbnailsInProcess(pool, thumbnailsJob, false, 0)
}

// ── Filesystem ────────────────────────────────────────────────────────────────

func (h *ImporterHandler) FilesystemStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := filesystemJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		RootDirectory     string `json:"root_directory"`
		MaxImages         *int   `json:"max_images"`
		ReferenceMode     bool   `json:"reference_mode"`
		IncludeSubfolders *bool  `json:"include_subfolders"`
		OverwriteExisting bool   `json:"overwrite_existing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	paths := strings.Split(req.RootDirectory, ";")
	var validPaths []string
	var invalidPaths []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if dirExists(p) {
			validPaths = append(validPaths, p)
		} else {
			invalidPaths = append(invalidPaths, p)
		}
	}
	if len(invalidPaths) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", strings.Join(invalidPaths, ", ")))
		return
	}
	if len(validPaths) == 0 {
		writeError(w, http.StatusBadRequest, "at least one directory path is required")
		return
	}

	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "Filesystem import not configured")
		return
	}

	filesystemJob.Start()
	filesystemJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting filesystem images import...",
		"current_file": nil, "files_processed": 0, "total_files": 0,
		"images_imported": 0, "images_referenced": 0, "images_updated": 0,
		"errors": 0, "error_messages": []string{}, "log_lines": []string{},
	})
	filesystemJob.AppendLogLine("Starting filesystem images import...")
	filesystemJob.Broadcast("status", map[string]any{"status_line": "Starting filesystem images import..."})

	maxImages := req.MaxImages
	if maxImages != nil && *maxImages <= 0 {
		maxImages = nil
	}
	includeSubfolders := req.IncludeSubfolders == nil || *req.IncludeSubfolders
	uid := appctx.UserIDFromCtx(r.Context())
	go runFilesystemInProcess(h.pool, filesystemJob, validPaths, h.excludePatterns, maxImages, req.ReferenceMode, includeSubfolders, req.OverwriteExisting, uid)

	writeJSON(w, map[string]any{
		"message":        "Filesystem images import started",
		"root_directory": req.RootDirectory,
	})
}

func runFilesystemInProcess(pool *sql.DB, job *importer.ImportJob, directories []string, excludePatterns []string, maxImages *int, referenceMode bool, includeSubfolders bool, overwriteExisting bool, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	storage := importstorage.NewImageStorage(pool)

	progressCallback := func(stats filesystemimport.ImportStats) {
		statusLine := fmt.Sprintf("Processing file %d of %d: %s | Imported: %d, Referenced: %d, Updated: %d, Errors: %d",
			stats.FilesProcessed, stats.TotalFiles, stats.CurrentFile,
			stats.ImagesImported, stats.ImagesReferenced, stats.ImagesUpdated, stats.Errors)
		job.AppendLogLine(statusLine)
		job.UpdateState(map[string]any{
			"total_files":       stats.TotalFiles,
			"files_processed":   stats.FilesProcessed,
			"images_imported":   stats.ImagesImported,
			"images_referenced": stats.ImagesReferenced,
			"images_updated":    stats.ImagesUpdated,
			"errors":            stats.Errors,
			"error_messages":    stats.ErrorMessages,
			"current_file":      stats.CurrentFile,
			"status_line":       statusLine,
		})
		if stats.FilesProcessed%25 == 0 {
			job.Broadcast("progress", job.GetState())
		}
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := filesystemimport.ImportImagesFromDirectories(ctx, storage, directories, excludePatterns, maxImages, referenceMode, includeSubfolders, overwriteExisting, progressCallback, cancelledCheck)

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

	statusLine := fmt.Sprintf("Completed: %d files processed, %d imported, %d referenced, %d updated, %d errors",
		stats.FilesProcessed, stats.ImagesImported, stats.ImagesReferenced, stats.ImagesUpdated, stats.Errors)
	job.UpdateState(map[string]any{
		"status":            "completed",
		"status_line":       statusLine,
		"total_files":       stats.TotalFiles,
		"files_processed":   stats.FilesProcessed,
		"images_imported":   stats.ImagesImported,
		"images_referenced": stats.ImagesReferenced,
		"images_updated":    stats.ImagesUpdated,
		"errors":            stats.Errors,
		"error_messages":    stats.ErrorMessages,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) FilesystemStream(w http.ResponseWriter, r *http.Request) {
	filesystemJob.ServeSSE(w, r)
}
func (h *ImporterHandler) FilesystemCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, filesystemJob.Cancel())
}
func (h *ImporterHandler) FilesystemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, filesystemJob.Status())
}

// ── Thumbnails ────────────────────────────────────────────────────────────────

func (h *ImporterHandler) ThumbnailsStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := thumbnailsJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "Thumbnail processing not configured")
		return
	}

	reprocess := r.URL.Query().Get("reprocess") == "true" || r.URL.Query().Get("reprocess") == "1"

	thumbnailsJob.Start()
	thumbnailsJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting thumbnail processing...",
		"phase": nil, "phase1_scanned": 0, "phase1_updated": 0,
		"phase2_scanned": 0, "phase2_total": 0, "phase2_processed": 0, "phase2_errors": 0,
	})
	thumbnailsJob.Broadcast("status", map[string]any{"status_line": "Starting thumbnail processing..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runThumbnailsInProcess(h.pool, thumbnailsJob, reprocess, uid)

	writeJSON(w, map[string]any{"message": "Thumbnail processing started", "status": "started"})
}

func (h *ImporterHandler) ThumbnailsStartAsync(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := thumbnailsJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "Thumbnail processing not configured")
		return
	}

	reprocess := r.URL.Query().Get("reprocess") == "true" || r.URL.Query().Get("reprocess") == "1"

	thumbnailsJob.Start()
	thumbnailsJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting thumbnail processing...",
		"phase": nil, "phase1_scanned": 0, "phase1_updated": 0,
		"phase2_scanned": 0, "phase2_total": 0, "phase2_processed": 0, "phase2_errors": 0,
	})
	thumbnailsJob.Broadcast("status", map[string]any{"status_line": "Starting thumbnail processing..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runThumbnailsInProcess(h.pool, thumbnailsJob, reprocess, uid)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "accepted"})
}

func runThumbnailsInProcess(pool *sql.DB, job *importer.ImportJob, reprocess bool, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	// Region updates before (backfill GPS rows missing region)
	_ = georegion.UpdateLocationRegions(ctx, pool)
	_ = georegion.UpdateMediaItemRegions(ctx, pool)

	progressCallback := func(stats thumbnailsimport.ImportStats) {
		statusLine := fmt.Sprintf("Processing: %d/%d items (%.1f%%) | Processed: %d | Errors: %d",
			int(stats.Processed), stats.TotalItems,
			float64(stats.Processed)/float64(max(1, stats.TotalItems))*100,
			stats.Processed, stats.Errors)
		job.UpdateState(map[string]any{
			"phase2_total":     stats.TotalItems,
			"phase2_processed": stats.Processed,
			"phase2_errors":    stats.Errors,
			"status_line":      statusLine,
		})
		job.Broadcast("progress", job.GetState())
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := thumbnailsimport.ProcessThumbnailsAndExif(ctx, pool, reprocess, progressCallback, cancelledCheck)

	// Region updates after (backfill GPS rows missing region)
	_ = georegion.UpdateLocationRegions(ctx, pool)
	_ = georegion.UpdateMediaItemRegions(ctx, pool)

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Processing cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}
	if err != nil {
		msg := fmt.Sprintf("import error: %s", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	statusLine := fmt.Sprintf("Completed: %d items processed, %d errors",
		stats.Processed, stats.Errors)
	job.UpdateState(map[string]any{
		"status":           "completed",
		"status_line":      statusLine,
		"phase2_total":     stats.TotalItems,
		"phase2_processed": stats.Processed,
		"phase2_errors":    stats.Errors,
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) ThumbnailsStream(w http.ResponseWriter, r *http.Request) {
	thumbnailsJob.ServeSSE(w, r)
}
func (h *ImporterHandler) ThumbnailsCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	res := thumbnailsJob.Cancel()
	// Immediate SSE + status so the UI reflects cancel before workers drain the queue.
	if thumbnailsJob.InProgress() && thumbnailsJob.IsCancelled() {
		thumbnailsJob.UpdateState(map[string]any{
			"status_line": "Cancellation requested; finishing images already in progress…",
		})
		thumbnailsJob.Broadcast("status", thumbnailsJob.GetState())
	}
	writeJSON(w, res)
}
func (h *ImporterHandler) ThumbnailsStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, thumbnailsJob.Status())
}

// ── Facebook Albums ───────────────────────────────────────────────────────────

// func (h *ImporterHandler) FacebookAlbumsStart(w http.ResponseWriter, r *http.Request) {
// 	if err := facebookAlbumsJob.AssertNotRunning(); err != nil {
// 		writeError(w, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	var req struct {
// 		DirectoryPath string `json:"directory_path"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		writeError(w, http.StatusBadRequest, "invalid JSON body")
// 		return
// 	}
// 	if !dirExists(req.DirectoryPath) {
// 		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
// 		return
// 	}

// 	facebookAlbumsJob.Start()
// 	facebookAlbumsJob.UpdateState(map[string]any{
// 		"status": "in_progress", "status_line": "Starting import-processor...",
// 		"albums_processed": 0, "total_albums": 0, "albums_imported": 0,
// 		"images_imported": 0, "images_found": 0, "images_missing": 0,
// 		"missing_image_filenames": []string{}, "errors": 0,
// 	})
// 	facebookAlbumsJob.Broadcast("status", map[string]any{"status_line": "Starting import-processor..."})

// 	runJob(facebookAlbumsJob, []string{"facebook-albums", "--path", req.DirectoryPath}, func(stdout string) {
// 		ap, ai, ii, ifound, imiss, errs, missing, msg := parseFacebookAlbumsStdout(stdout)
// 		facebookAlbumsJob.UpdateState(map[string]any{
// 			"status": "completed", "status_line": msg,
// 			"albums_processed": ap, "total_albums": ap, "albums_imported": ai,
// 			"images_imported": ii, "images_found": ifound, "images_missing": imiss,
// 			"missing_image_filenames": missing, "errors": errs,
// 		})
// 		runThumbnailsAfterImportIfIdle(h.pool)
// 		facebookAlbumsJob.Broadcast("completed", facebookAlbumsJob.GetState())
// 	})

// 	writeJSON(w, map[string]any{"message": "Facebook Albums import started", "directory_path": req.DirectoryPath})
// }

// func (h *ImporterHandler) FacebookAlbumsStream(w http.ResponseWriter, r *http.Request) {
// 	facebookAlbumsJob.ServeSSE(w, r)
// }
// func (h *ImporterHandler) FacebookAlbumsCancel(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookAlbumsJob.Cancel())
// }
// func (h *ImporterHandler) FacebookAlbumsStatus(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookAlbumsJob.Status())
// }

// // ── Facebook Posts ────────────────────────────────────────────────────────────

// func (h *ImporterHandler) FacebookPostsStart(w http.ResponseWriter, r *http.Request) {
// 	if err := facebookPostsJob.AssertNotRunning(); err != nil {
// 		writeError(w, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	var req struct {
// 		DirectoryPath string `json:"directory_path"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		writeError(w, http.StatusBadRequest, "invalid JSON body")
// 		return
// 	}
// 	if !dirExists(req.DirectoryPath) {
// 		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
// 		return
// 	}

// 	facebookPostsJob.Start()
// 	facebookPostsJob.UpdateState(map[string]any{
// 		"status": "in_progress", "status_line": "Starting import-processor...",
// 		"posts_processed": 0, "posts_imported": 0, "posts_updated": 0,
// 		"with_media": 0, "images_imported": 0, "images_found": 0, "images_missing": 0, "errors": 0,
// 	})
// 	facebookPostsJob.Broadcast("status", map[string]any{"status_line": "Starting import-processor..."})

// 	runJob(facebookPostsJob, []string{"facebook-posts", "--path", req.DirectoryPath}, func(stdout string) {
// 		stats := parseFacebookPostsStdout(stdout)
// 		stats["status"] = "completed"
// 		stats["status_line"] = "Import completed"
// 		facebookPostsJob.UpdateState(stats)
// 		runThumbnailsAfterImportIfIdle(h.pool)
// 		facebookPostsJob.Broadcast("completed", facebookPostsJob.GetState())
// 	})

// 	writeJSON(w, map[string]any{"message": "Facebook Posts import started", "directory_path": req.DirectoryPath})
// }

// func (h *ImporterHandler) FacebookPostsStream(w http.ResponseWriter, r *http.Request) {
// 	facebookPostsJob.ServeSSE(w, r)
// }
// func (h *ImporterHandler) FacebookPostsCancel(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookPostsJob.Cancel())
// }
// func (h *ImporterHandler) FacebookPostsStatus(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookPostsJob.Status())
// }

// // ── Facebook Places ───────────────────────────────────────────────────────────

// func (h *ImporterHandler) FacebookPlacesStart(w http.ResponseWriter, r *http.Request) {
// 	if err := facebookPlacesJob.AssertNotRunning(); err != nil {
// 		writeError(w, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	var req struct {
// 		DirectoryPath string `json:"directory_path"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		writeError(w, http.StatusBadRequest, "invalid JSON body")
// 		return
// 	}
// 	if !dirExists(req.DirectoryPath) {
// 		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
// 		return
// 	}

// 	facebookPlacesJob.Start()
// 	facebookPlacesJob.UpdateState(map[string]any{
// 		"status": "in_progress", "status_line": "Starting import-processor...",
// 		"places_imported": 0, "places_created": 0, "places_updated": 0,
// 	})
// 	facebookPlacesJob.Broadcast("status", map[string]any{"status_line": "Starting import-processor..."})

// 	runJob(facebookPlacesJob, []string{"facebook-places", "--path", req.DirectoryPath}, func(stdout string) {
// 		stats := parseFacebookPlacesStdout(stdout)
// 		stats["status"] = "completed"
// 		stats["status_line"] = "Import completed"
// 		facebookPlacesJob.UpdateState(stats)
// 		facebookPlacesJob.Broadcast("completed", facebookPlacesJob.GetState())
// 	})

// 	writeJSON(w, map[string]any{"message": "Facebook Places import started", "directory_path": req.DirectoryPath})
// }

// func (h *ImporterHandler) FacebookPlacesStream(w http.ResponseWriter, r *http.Request) {
// 	facebookPlacesJob.ServeSSE(w, r)
// }
// func (h *ImporterHandler) FacebookPlacesCancel(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookPlacesJob.Cancel())
// }
// func (h *ImporterHandler) FacebookPlacesStatus(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookPlacesJob.Status())
// }

// ── Facebook All ──────────────────────────────────────────────────────────────

func (h *ImporterHandler) FacebookAllStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := facebookAllJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		DirectoryPath string  `json:"directory_path"`
		UserName      *string `json:"user_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !dirExists(req.DirectoryPath) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("path does not exist: %s", req.DirectoryPath))
		return
	}

	facebookAllJob.Start()
	facebookAllJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting Facebook All import...",
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "att_found": 0, "att_missing": 0, "messenger_errors": 0,
		"albums_processed": 0, "albums_imported": 0, "album_images_imported": 0,
		"album_images_found": 0, "album_images_missing": 0, "albums_errors": 0,
		"places_imported": 0, "places_created": 0, "places_updated": 0,
		"posts_processed": 0, "posts_imported": 0, "posts_updated": 0,
		"with_media": 0, "images_imported": 0, "images_found": 0,
		"images_missing": 0, "posts_errors": 0,
	})
	facebookAllJob.Broadcast("status", map[string]any{"status_line": "Starting Facebook All import..."})

	userName := ""
	if req.UserName != nil && *req.UserName != "" {
		userName = strings.TrimSpace(*req.UserName)
	}
	uid := appctx.UserIDFromCtx(r.Context())
	go runFacebookAllInProcess(h.pool, h.subjectConfigRepo, facebookAllJob, req.DirectoryPath, userName, uid)

	writeJSON(w, map[string]any{"message": "Facebook All import started", "directory_path": req.DirectoryPath})
}

func (h *ImporterHandler) FacebookAllStream(w http.ResponseWriter, r *http.Request) {
	facebookAllJob.ServeSSE(w, r)
}
func (h *ImporterHandler) FacebookAllCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, facebookAllJob.Cancel())
}
func (h *ImporterHandler) FacebookAllStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, facebookAllJob.Status())
}

// ── Contacts extract ──────────────────────────────────────────────────────────

func (h *ImporterHandler) ContactsExtractStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := contactsExtractJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	contactsExtractJob.Start()
	contactsExtractJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting contacts extract...",
		"contacts_processed": 0, "contacts_merged": 0, "contacts_created": 0,
	})
	contactsExtractJob.Broadcast("status", map[string]any{"status_line": "Starting contacts extract..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runContactsExtractInProcess(h.pool, contactsExtractJob, uid)

	writeJSON(w, map[string]any{"message": "Contacts extract started", "status": "started"})
}

func runContactsExtractInProcess(pool *sql.DB, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	progressFunc := func(msg string) {
		job.UpdateState(map[string]any{"status_line": msg})
		job.Broadcast("progress", job.GetState())
	}

	opts := contactsimport.RunOptions{
		Workers:           runtime.NumCPU(),
		ContactsDB:        pool,
		RelationshipQuery: os.Getenv("CONTACTS_RELATIONSHIP_QUERY"),
		ProgressFunc:      progressFunc,
		OwnerUserID:       uid,
	}

	if err := contactsimport.RunContactsNormalise(ctx, opts); err != nil {
		if job.IsCancelled() {
			job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Contacts extract cancelled."})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		msg := fmt.Sprintf("contacts extract error: %s", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": "Contacts extract completed successfully.",
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) ContactsExtractStream(w http.ResponseWriter, r *http.Request) {
	contactsExtractJob.ServeSSE(w, r)
}
func (h *ImporterHandler) ContactsExtractCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, contactsExtractJob.Cancel())
}
func (h *ImporterHandler) ContactsExtractStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, contactsExtractJob.Status())
}

// ── Reference import ──────────────────────────────────────────────────────────

func (h *ImporterHandler) ReferenceImportStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := referenceImportJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.imageRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "reference import not configured")
		return
	}

	referenceImportJob.Start()
	referenceImportJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting reference import...",
		"total": 0, "processed": 0, "imported": 0, "skipped": 0, "errors": 0,
		"error_message": nil, "error_messages": []string{},
	})
	referenceImportJob.Broadcast("status", map[string]any{"status_line": "Starting reference import..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runReferenceImport(h.imageRepo, referenceImportJob, h.pool, uid)

	writeJSON(w, map[string]any{"message": "Reference import started", "status": "started"})
}

func runReferenceImport(repo *repository.ImageRepo, job *importer.ImportJob, pool *sql.DB, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	items, err := repo.ListReferencedItems(ctx)
	if err != nil {
		job.UpdateState(map[string]any{"status": "error", "error_message": err.Error(), "status_line": err.Error()})
		job.Broadcast("error", job.GetState())
		return
	}
	total := len(items)
	job.UpdateState(map[string]any{"total": total, "status_line": fmt.Sprintf("Found %d referenced images to import", total)})
	job.Broadcast("progress", job.GetState())

	imported, skipped, errCount := 0, 0, 0
	var errMsgs []string
	for i, it := range items {
		if job.IsCancelled() {
			job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Processing cancelled.", "processed": i, "imported": imported, "skipped": skipped, "errors": errCount, "error_messages": errMsgs})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		data, err := os.ReadFile(it.SourceReference)
		if err != nil {
			skipped++
			msg := fmt.Sprintf("File not found: %s", it.SourceReference)
			errMsgs = append(errMsgs, msg)
		} else if err := repo.UpdateBlobImageDataAndClearReferenced(ctx, it.ID, it.MediaBlobID, data); err != nil {
			errCount++
			errMsgs = append(errMsgs, fmt.Sprintf("Failed to update media_item %d: %v", it.ID, err))
		} else {
			imported++
		}
		job.UpdateState(map[string]any{
			"processed": i + 1, "imported": imported, "skipped": skipped, "errors": errCount,
			"error_messages": errMsgs,
			"status_line":    fmt.Sprintf("Item %d/%d: %d imported, %d skipped, %d errors", i+1, total, imported, skipped, errCount),
		})
		job.Broadcast("progress", job.GetState())
	}

	statusLine := fmt.Sprintf("Completed: %d imported, %d skipped, %d errors", imported, skipped, errCount)
	var errMsg any
	if len(errMsgs) > 0 {
		end := 5
		if len(errMsgs) < end {
			end = len(errMsgs)
		}
		errMsg = strings.Join(errMsgs[:end], "; ")
	}
	job.UpdateState(map[string]any{
		"status": "completed", "status_line": statusLine,
		"processed": total, "imported": imported, "skipped": skipped, "errors": errCount,
		"error_message": errMsg, "error_messages": errMsgs,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) ReferenceImportStream(w http.ResponseWriter, r *http.Request) {
	referenceImportJob.ServeSSE(w, r)
}
func (h *ImporterHandler) ReferenceImportCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, referenceImportJob.Cancel())
}
func (h *ImporterHandler) ReferenceImportStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, referenceImportJob.Status())
}

// ── Image region recalculation ────────────────────────────────────────────────

func (h *ImporterHandler) ImageRegionsRecalcStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := imageRegionsRecalcJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "region recalculation not configured")
		return
	}

	imageRegionsRecalcJob.Start()
	imageRegionsRecalcJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting region recalculation...",
		"total": 0, "processed": 0, "updated": 0, "error_message": nil,
	})
	imageRegionsRecalcJob.Broadcast("status", map[string]any{"status_line": "Starting region recalculation..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runImageRegionsRecalc(h.pool, imageRegionsRecalcJob, uid)

	writeJSON(w, map[string]any{"message": "Region recalculation started", "status": "started"})
}

func runImageRegionsRecalc(pool *sql.DB, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	countGPS := func() int {
		q := `SELECT COUNT(id) FROM media_items
		      WHERE media_type LIKE 'image/%'
		        AND latitude IS NOT NULL AND longitude IS NOT NULL`
		args := []any{}
		if uid > 0 {
			q += " AND user_id = ?"
			args = append(args, uid)
		}
		var total int
		if err := pool.QueryRowContext(ctx, q, args...).Scan(&total); err != nil {
			return 0
		}
		return total
	}

	total := countGPS()
	job.UpdateState(map[string]any{
		"total": total,
		"status_line": fmt.Sprintf("Recalculating regions for %d GPS-tagged images", total),
	})
	job.Broadcast("progress", job.GetState())

	if total == 0 {
		statusLine := "Completed: no GPS-tagged images to update"
		job.UpdateState(map[string]any{
			"status": "completed", "status_line": statusLine,
			"processed": 0, "updated": 0, "error_message": nil,
		})
		job.Broadcast("completed", job.GetState())
		return
	}

	updated, err := georegion.RecalculateImageRegions(ctx, pool, &georegion.RegionRecalcOptions{
		Progress: func(processed, total int) {
			job.UpdateState(map[string]any{
				"processed": processed,
				"updated":   processed,
				"status_line": fmt.Sprintf("Item %d/%d: recalculating regions", processed, total),
			})
			job.Broadcast("progress", job.GetState())
		},
		Cancel: job.IsCancelled,
	})
	if err != nil {
		if errors.Is(err, georegion.ErrRegionRecalcCancelled) {
			job.UpdateState(map[string]any{
				"status": "cancelled", "status_line": "Processing cancelled.",
				"processed": updated, "updated": updated,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		job.UpdateState(map[string]any{
			"status": "error", "error_message": err.Error(), "status_line": err.Error(),
		})
		job.Broadcast("error", job.GetState())
		return
	}

	statusLine := fmt.Sprintf("Completed: %d regions recalculated", updated)
	job.UpdateState(map[string]any{
		"status": "completed", "status_line": statusLine,
		"processed": updated, "updated": updated, "error_message": nil,
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) ImageRegionsRecalcStream(w http.ResponseWriter, r *http.Request) {
	imageRegionsRecalcJob.ServeSSE(w, r)
}

func (h *ImporterHandler) ImageRegionsRecalcCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, imageRegionsRecalcJob.Cancel())
}

func (h *ImporterHandler) ImageRegionsRecalcStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imageRegionsRecalcJob.Status())
}

// ── Media classification tag QA ───────────────────────────────────────────────

const classificationTagQAMinTags = 5

func (h *ImporterHandler) MediaClassificationTagQAStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := mediaClassificationTagQAJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.imageRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "media classification tag QA not configured")
		return
	}

	mediaClassificationTagQAJob.Start()
	mediaClassificationTagQAJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting media classification tag QA...",
		"total": 0, "processed": 0, "reflagged": 0, "errors": 0, "error_message": nil,
	})
	mediaClassificationTagQAJob.Broadcast("status", map[string]any{"status_line": "Starting media classification tag QA..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runMediaClassificationTagQA(h.imageRepo, mediaClassificationTagQAJob, uid)

	writeJSON(w, map[string]any{"message": "Media classification tag QA started", "status": "started"})
}

func runMediaClassificationTagQA(repo *repository.ImageRepo, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	rows, err := repo.ListMediaItemsForClassificationTagQA(ctx)
	if err != nil {
		msg := fmt.Sprintf("list classified media items: %v", err)
		job.UpdateState(map[string]any{
			"status": "error", "error_message": msg, "status_line": msg,
		})
		job.Broadcast("error", job.GetState())
		return
	}

	total := len(rows)
	job.UpdateState(map[string]any{
		"total":       total,
		"status_line": fmt.Sprintf("Checking tag counts on %d classified image(s)", total),
	})
	job.Broadcast("progress", job.GetState())

	if total == 0 {
		statusLine := "Completed: no classified images to check"
		job.UpdateState(map[string]any{
			"status": "completed", "status_line": statusLine,
			"processed": 0, "reflagged": 0, "errors": 0, "error_message": nil,
		})
		job.Broadcast("completed", job.GetState())
		return
	}

	var processed, reflagged, errs int
	for _, row := range rows {
		if job.IsCancelled() {
			statusLine := fmt.Sprintf("Cancelled after checking %d/%d image(s); re-flagged %d", processed, total, reflagged)
			job.UpdateState(map[string]any{
				"status": "cancelled", "status_line": statusLine,
				"processed": processed, "reflagged": reflagged, "errors": errs,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}

		processed++
		tags := ""
		if row.Tags != nil {
			tags = *row.Tags
		}
		if repository.CountCommaSeparatedTags(tags) < classificationTagQAMinTags {
			ok, setErr := repo.SetRequireClassification(ctx, row.ID, true)
			if setErr != nil {
				errs++
			} else if ok {
				reflagged++
			}
		}

		if processed == total || processed%50 == 0 {
			job.UpdateState(map[string]any{
				"processed":   processed,
				"reflagged":   reflagged,
				"errors":      errs,
				"status_line": fmt.Sprintf("Checked %d/%d image(s); re-flagged %d", processed, total, reflagged),
			})
			job.Broadcast("progress", job.GetState())
		}
	}

	statusLine := fmt.Sprintf("Completed: checked %d image(s), re-flagged %d with fewer than %d tags", processed, reflagged, classificationTagQAMinTags)
	if errs > 0 {
		statusLine += fmt.Sprintf(" (%d errors)", errs)
	}
	job.UpdateState(map[string]any{
		"status": "completed", "status_line": statusLine,
		"processed": processed, "reflagged": reflagged, "errors": errs, "error_message": nil,
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) MediaClassificationTagQAStream(w http.ResponseWriter, r *http.Request) {
	mediaClassificationTagQAJob.ServeSSE(w, r)
}

func (h *ImporterHandler) MediaClassificationTagQACancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, mediaClassificationTagQAJob.Cancel())
}

func (h *ImporterHandler) MediaClassificationTagQAStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, mediaClassificationTagQAJob.Status())
}

// ── Media GPS duplicate spread ─────────────────────────────────────────────────

func (h *ImporterHandler) MediaGPSDuplicateSpreadStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := mediaGPSDuplicateSpreadJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.imageRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "GPS duplicate spread not configured")
		return
	}

	mediaGPSDuplicateSpreadJob.Start()
	mediaGPSDuplicateSpreadJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting duplicate GPS spread...",
		"total": 0, "processed": 0, "updated": 0, "errors": 0, "error_message": nil,
	})
	mediaGPSDuplicateSpreadJob.Broadcast("status", map[string]any{"status_line": "Starting duplicate GPS spread..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runMediaGPSDuplicateSpread(h.imageRepo, mediaGPSDuplicateSpreadJob, uid)

	writeJSON(w, map[string]any{"message": "Duplicate GPS spread started", "status": "started"})
}

func runMediaGPSDuplicateSpread(repo *repository.ImageRepo, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	job.UpdateState(map[string]any{
		"status_line": "Scanning for duplicate GPS groups...",
	})
	job.Broadcast("progress", job.GetState())

	groups, err := repo.ListDuplicateImageGPSGroups(ctx)
	if err != nil {
		msg := fmt.Sprintf("list duplicate GPS groups: %v", err)
		job.UpdateState(map[string]any{
			"status": "error", "error_message": msg, "status_line": msg,
		})
		job.Broadcast("error", job.GetState())
		return
	}

	total := len(groups)
	job.UpdateState(map[string]any{
		"total":       total,
		"status_line": fmt.Sprintf("Spreading %d duplicate GPS group(s)", total),
	})
	job.Broadcast("progress", job.GetState())

	if total == 0 {
		statusLine := "Completed: no duplicate GPS groups found"
		job.UpdateState(map[string]any{
			"status": "completed", "status_line": statusLine,
			"processed": 0, "updated": 0, "errors": 0, "error_message": nil,
		})
		job.Broadcast("completed", job.GetState())
		return
	}

	var processed, updated, errs int
	for _, group := range groups {
		if job.IsCancelled() {
			statusLine := fmt.Sprintf("Cancelled after %d/%d group(s); %d image(s) spread", processed, total, updated)
			job.UpdateState(map[string]any{
				"status": "cancelled", "status_line": statusLine,
				"processed": processed, "updated": updated, "errors": errs,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}

		radius := repository.GPSDuplicateSpreadRadiusForCount(len(group.IDs))
		n, spreadErr := repo.SpreadImageGPSOnCircle(ctx, group.IDs, group.Latitude, group.Longitude, radius)
		processed++
		if spreadErr != nil {
			errs++
		} else {
			updated += n
		}

		job.UpdateState(map[string]any{
			"processed":   processed,
			"updated":     updated,
			"errors":      errs,
			"status_line": fmt.Sprintf("Group: %d/%d | %d image(s) spread | %d errors", processed, total, updated, errs),
		})
		job.Broadcast("progress", job.GetState())
	}

	statusLine := fmt.Sprintf("Completed: %d group(s), %d image(s) spread", processed, updated)
	if errs > 0 {
		statusLine += fmt.Sprintf(" (%d errors)", errs)
	}
	job.UpdateState(map[string]any{
		"status": "completed", "status_line": statusLine,
		"processed": processed, "updated": updated, "errors": errs, "error_message": nil,
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) MediaGPSDuplicateSpreadStream(w http.ResponseWriter, r *http.Request) {
	mediaGPSDuplicateSpreadJob.ServeSSE(w, r)
}

func (h *ImporterHandler) MediaGPSDuplicateSpreadCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, mediaGPSDuplicateSpreadJob.Cancel())
}

func (h *ImporterHandler) MediaGPSDuplicateSpreadStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, mediaGPSDuplicateSpreadJob.Status())
}

// ── Image export ───────────────────────────────────────────────────────────────

func (h *ImporterHandler) ImageExportStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := imageExportJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.imageRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "image export not configured")
		return
	}
	var req struct {
		TargetDirectory string `json:"target_directory"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	targetDir := strings.TrimSpace(req.TargetDirectory)
	if targetDir == "" {
		writeError(w, http.StatusBadRequest, "target_directory is required")
		return
	}

	imageExportJob.Start()
	imageExportJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting image export...",
		"total": 0, "processed": 0, "exported": 0, "skipped": 0, "errors": 0,
		"error_message": nil, "error_messages": []string{},
	})
	imageExportJob.Broadcast("status", map[string]any{"status_line": "Starting image export..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runImageExport(h.imageRepo, imageExportJob, targetDir, uid)

	writeJSON(w, map[string]any{"message": "Image export started", "status": "started"})
}

func runImageExport(repo *repository.ImageRepo, job *importer.ImportJob, targetDir string, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		job.UpdateState(map[string]any{"status": "error", "error_message": err.Error(), "status_line": err.Error()})
		job.Broadcast("error", job.GetState())
		return
	}

	items, err := repo.ListMediaItemsForExport(ctx)
	if err != nil {
		job.UpdateState(map[string]any{"status": "error", "error_message": err.Error(), "status_line": err.Error()})
		job.Broadcast("error", job.GetState())
		return
	}
	total := len(items)
	job.UpdateState(map[string]any{"total": total, "status_line": fmt.Sprintf("Found %d images to export", total)})
	job.Broadcast("progress", job.GetState())

	exported, skipped, errCount := 0, 0, 0
	var errMsgs []string
	subdirCount := 0
	const maxPerDir = 200

	for i, it := range items {
		if job.IsCancelled() {
			job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Export cancelled.", "processed": i, "exported": exported, "skipped": skipped, "errors": errCount, "error_messages": errMsgs})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		data, err := repo.GetBlobImageData(ctx, it.MediaBlobID)
		if err != nil || len(data) == 0 {
			skipped++
			errMsgs = append(errMsgs, fmt.Sprintf("Image %d: no data", it.ID))
		} else {
			subIdx := i / maxPerDir
			subdir := filepath.Join(targetDir, fmt.Sprintf("%04d", subIdx))
			if subIdx > subdirCount {
				_ = os.MkdirAll(subdir, 0755)
				subdirCount = subIdx
			}
			ext := extForMediaType(it.MediaType)
			filename := fmt.Sprintf("%d.%s", it.ID, ext)
			if it.SourceRef != nil && *it.SourceRef != "" {
				ext2 := filepath.Ext(*it.SourceRef)
				if ext2 != "" {
					filename = fmt.Sprintf("%d%s", it.ID, ext2)
				}
			}
			path := filepath.Join(subdir, filename)
			if err := os.WriteFile(path, data, 0644); err != nil {
				errCount++
				errMsgs = append(errMsgs, fmt.Sprintf("Image %d: %v", it.ID, err))
			} else {
				exported++
			}
		}
		job.UpdateState(map[string]any{
			"processed": i + 1, "exported": exported, "skipped": skipped, "errors": errCount,
			"error_messages": errMsgs,
			"status_line":    fmt.Sprintf("Item %d/%d: %d exported, %d skipped, %d errors", i+1, total, exported, skipped, errCount),
		})
		job.Broadcast("progress", job.GetState())
	}

	statusLine := fmt.Sprintf("Completed: %d exported, %d skipped, %d errors", exported, skipped, errCount)
	var errMsg any
	if len(errMsgs) > 0 {
		end := 5
		if len(errMsgs) < end {
			end = len(errMsgs)
		}
		errMsg = strings.Join(errMsgs[:end], "; ")
	}
	job.UpdateState(map[string]any{
		"status": "completed", "status_line": statusLine,
		"processed": total, "exported": exported, "skipped": skipped, "errors": errCount,
		"error_message": errMsg, "error_messages": errMsgs,
	})
	job.Broadcast("completed", job.GetState())
}

func extForMediaType(mt *string) string {
	if mt == nil {
		return "jpg"
	}
	switch strings.ToLower(*mt) {
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/heic", "image/heif":
		return "heic"
	case "image/bmp":
		return "bmp"
	case "image/tiff":
		return "tiff"
	default:
		return "jpg"
	}
}

func (h *ImporterHandler) ImageExportStream(w http.ResponseWriter, r *http.Request) {
	imageExportJob.ServeSSE(w, r)
}
func (h *ImporterHandler) ImageExportCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, imageExportJob.Cancel())
}
func (h *ImporterHandler) ImageExportStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imageExportJob.Status())
}

// ── Image metadata JSON export ────────────────────────────────────────────────

func (h *ImporterHandler) ImageMetadataJSONExportStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := imageMetadataJSONExportJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.imageRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "image metadata JSON export not configured")
		return
	}
	var req struct {
		OutputPath string `json:"output_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	outputPath := strings.TrimSpace(req.OutputPath)
	if outputPath == "" {
		writeError(w, http.StatusBadRequest, "output_path is required")
		return
	}

	imageMetadataJSONExportJob.Start()
	imageMetadataJSONExportJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting image metadata JSON export...",
		"total": 0, "processed": 0, "exported": 0, "errors": 0, "error_message": nil,
	})
	imageMetadataJSONExportJob.Broadcast("status", map[string]any{"status_line": "Starting image metadata JSON export..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runImageMetadataJSONExport(h.imageRepo, imageMetadataJSONExportJob, outputPath, uid)

	writeJSON(w, map[string]any{"message": "Image metadata JSON export started", "status": "started"})
}

func runImageMetadataJSONExport(repo *repository.ImageRepo, job *importer.ImportJob, outputPath string, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	job.UpdateState(map[string]any{"status_line": "Loading image metadata..."})
	job.Broadcast("progress", job.GetState())

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Export cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	records, err := repo.ListImageMetadataForJSONExport(ctx)
	if err != nil {
		msg := fmt.Sprintf("load image metadata: %v", err)
		job.UpdateState(map[string]any{"status": "error", "error_message": msg, "status_line": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	total := len(records)
	job.UpdateState(map[string]any{
		"total":       total,
		"processed":   total,
		"status_line": fmt.Sprintf("Writing %d image(s) to JSON file", total),
	})
	job.Broadcast("progress", job.GetState())

	if job.IsCancelled() {
		job.UpdateState(map[string]any{
			"status": "cancelled", "status_line": "Export cancelled.",
			"processed": total, "exported": 0,
		})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		msg := fmt.Sprintf("create output directory: %v", err)
		job.UpdateState(map[string]any{"status": "error", "error_message": msg, "status_line": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		msg := fmt.Sprintf("encode JSON: %v", err)
		job.UpdateState(map[string]any{"status": "error", "error_message": msg, "status_line": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		msg := fmt.Sprintf("write JSON file: %v", err)
		job.UpdateState(map[string]any{"status": "error", "error_message": msg, "status_line": msg, "errors": 1})
		job.Broadcast("error", job.GetState())
		return
	}

	statusLine := fmt.Sprintf("Completed: wrote %d image(s) to %s", total, outputPath)
	job.UpdateState(map[string]any{
		"status": "completed", "status_line": statusLine,
		"processed": total, "exported": total, "errors": 0, "error_message": nil,
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) ImageMetadataJSONExportStream(w http.ResponseWriter, r *http.Request) {
	imageMetadataJSONExportJob.ServeSSE(w, r)
}

func (h *ImporterHandler) ImageMetadataJSONExportCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, imageMetadataJSONExportJob.Cancel())
}

func (h *ImporterHandler) ImageMetadataJSONExportStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imageMetadataJSONExportJob.Status())
}

// ── WhatsApp ──────────────────────────────────────────────────────────────────

func (h *ImporterHandler) WhatsAppStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := whatsappJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		DirectoryPath string `json:"directory_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !dirExists(req.DirectoryPath) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
		return
	}
	if h.pool == nil || h.subjectConfigRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp import not configured")
		return
	}

	whatsappJob.Start()
	whatsappJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting WhatsApp import...",
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "attachments_found": 0, "attachments_missing": 0,
		"missing_attachment_filenames": []string{}, "errors": 0,
	})
	whatsappJob.Broadcast("status", map[string]any{"status_line": "Starting WhatsApp import..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runWhatsAppInProcess(h.pool, h.subjectConfigRepo, whatsappJob, req.DirectoryPath, uid)

	writeJSON(w, map[string]any{"message": "WhatsApp import started", "directory_path": req.DirectoryPath})
}

func runWhatsAppInProcess(pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath string, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats whatsappimport.ImportStats, justCompleted string) {
		// statusLine := fmt.Sprintf("Processing conversation %d of %d: %s | Total Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
		// 	stats.ConversationsProcessed, stats.TotalConversations, justCompleted,
		// 	stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
		// 	stats.AttachmentsFound,stats.AttachmentsMissing, stats.Errors)
		statusLine := fmt.Sprintf("Processing conversation %d of %d: %s | Total Messages: %d (%d created, %d updated) | Attachments: %d found",
			stats.ConversationsProcessed, stats.TotalConversations, justCompleted,
			stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
			stats.AttachmentsFound)
		job.UpdateState(map[string]any{
			"conversations":                stats.ConversationsProcessed,
			"messages_imported":            stats.MessagesImported,
			"messages_created":             stats.MessagesCreated,
			"messages_updated":             stats.MessagesUpdated,
			"attachments_found":            stats.AttachmentsFound,
			"attachments_missing":          stats.AttachmentsMissing,
			"missing_attachment_filenames": stats.MissingAttachmentFilenames,
			"errors":                       stats.Errors,
			"status_line":                  statusLine,
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

	// Final state from ImportStats
	statusLine := fmt.Sprintf("Completed: %d conversations, %d messages (%d created, %d updated), %d attachments found, %d missing",
		stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
		stats.AttachmentsFound, stats.AttachmentsMissing)
	job.UpdateState(map[string]any{
		"status":                       "completed",
		"status_line":                  statusLine,
		"conversations":                stats.ConversationsProcessed,
		"messages_imported":            stats.MessagesImported,
		"messages_created":             stats.MessagesCreated,
		"messages_updated":             stats.MessagesUpdated,
		"attachments_found":            stats.AttachmentsFound,
		"attachments_missing":          stats.AttachmentsMissing,
		"missing_attachment_filenames": stats.MissingAttachmentFilenames,
		"errors":                       stats.Errors,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) WhatsAppStream(w http.ResponseWriter, r *http.Request) {
	whatsappJob.ServeSSE(w, r)
}
func (h *ImporterHandler) WhatsAppCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, whatsappJob.Cancel())
}
func (h *ImporterHandler) WhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, whatsappJob.Status())
}

// ── iMessage ──────────────────────────────────────────────────────────────────

func (h *ImporterHandler) IMessageStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := imessageJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		DirectoryPath string `json:"directory_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !dirExists(req.DirectoryPath) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
		return
	}
	if h.pool == nil || h.subjectConfigRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "iMessage import not configured")
		return
	}

	imessageJob.Start()
	imessageJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting iMessage import...",
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "attachments_found": 0, "attachments_missing": 0,
		"missing_attachment_filenames": []string{}, "errors": 0,
	})
	imessageJob.Broadcast("status", map[string]any{"status_line": "Starting iMessage import..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runIMessageInProcess(h.pool, h.subjectConfigRepo, imessageJob, req.DirectoryPath, uid)

	writeJSON(w, map[string]any{"message": "iMessage import started", "directory_path": req.DirectoryPath})
}

func runIMessageInProcess(pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath string, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats imessageimport.ImportStats) {
		statusLine := ""
		if stats.TotalConversations > 0 {
			// statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Placeholders: %d, orphan imports: %d | Errors: %d",
			// 	stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
			// 	stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
			// 	stats.AttachmentsFound, stats.AttachmentsMissing,
			// 	stats.PlaceholdersUsed, stats.OrphanAttachmentsImported, stats.Errors)
			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Placeholders: %d, orphan imports: %d ",
				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
				stats.AttachmentsFound, stats.AttachmentsMissing,
				stats.PlaceholdersUsed, stats.OrphanAttachmentsImported)

		}
		job.UpdateState(map[string]any{
			"conversations":                stats.ConversationsProcessed,
			"messages_imported":            stats.MessagesImported,
			"messages_created":             stats.MessagesCreated,
			"messages_updated":             stats.MessagesUpdated,
			"attachments_found":            stats.AttachmentsFound,
			"attachments_missing":          stats.AttachmentsMissing,
			"placeholders_used":            stats.PlaceholdersUsed,
			"orphan_attachments_imported":  stats.OrphanAttachmentsImported,
			"missing_attachment_filenames": stats.MissingAttachmentFilenames,
			"errors":                       stats.Errors,
			"status_line":                  statusLine,
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

	statusLine := fmt.Sprintf("Completed: %d conversations, %d messages (%d created, %d updated), %d attachments found, %d missing, %d placeholders, %d orphan attachments",
		stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
		stats.AttachmentsFound, stats.AttachmentsMissing, stats.PlaceholdersUsed, stats.OrphanAttachmentsImported)
	job.UpdateState(map[string]any{
		"status":                       "completed",
		"status_line":                  statusLine,
		"conversations":                stats.ConversationsProcessed,
		"messages_imported":            stats.MessagesImported,
		"messages_created":             stats.MessagesCreated,
		"messages_updated":             stats.MessagesUpdated,
		"attachments_found":            stats.AttachmentsFound,
		"attachments_missing":          stats.AttachmentsMissing,
		"placeholders_used":            stats.PlaceholdersUsed,
		"orphan_attachments_imported":  stats.OrphanAttachmentsImported,
		"missing_attachment_filenames": stats.MissingAttachmentFilenames,
		"errors":                       stats.Errors,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) IMessageStream(w http.ResponseWriter, r *http.Request) {
	imessageJob.ServeSSE(w, r)
}
func (h *ImporterHandler) IMessageCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, imessageJob.Cancel())
}
func (h *ImporterHandler) IMessageStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imessageJob.Status())
}

// ── Instagram ─────────────────────────────────────────────────────────────────

func (h *ImporterHandler) InstagramStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := instagramJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		DirectoryPath string  `json:"directory_path"`
		UserName      *string `json:"user_name"`
		ExportRoot    *string `json:"export_root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !dirExists(req.DirectoryPath) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
		return
	}
	if h.pool == nil || h.subjectConfigRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "Instagram import not configured")
		return
	}

	instagramJob.Start()
	instagramJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting Instagram import...",
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "errors": 0,
	})
	instagramJob.Broadcast("status", map[string]any{"status_line": "Starting Instagram import..."})

	userName := ""
	if req.UserName != nil && *req.UserName != "" {
		userName = *req.UserName
	}
	exportRoot := ""
	if req.ExportRoot != nil && *req.ExportRoot != "" {
		exportRoot = *req.ExportRoot
	}
	uid := appctx.UserIDFromCtx(r.Context())
	go runInstagramInProcess(h.pool, h.subjectConfigRepo, instagramJob, req.DirectoryPath, userName, exportRoot, uid)

	writeJSON(w, map[string]any{"message": "Instagram import started", "directory_path": req.DirectoryPath})
}

func runInstagramInProcess(pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath, userName, exportRoot string, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
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
			"conversations":     stats.ConversationsProcessed,
			"messages_imported": stats.MessagesImported,
			"messages_created":  stats.MessagesCreated,
			"messages_updated":  stats.MessagesUpdated,
			"errors":            stats.Errors,
			"status_line":       statusLine,
		})
		job.Broadcast("progress", job.GetState())
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := instagramimport.ImportInstagramFromDirectory(ctx, storage, directoryPath, progressCallback, cancelledCheck, exportRoot, userName)

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
		"status":            "completed",
		"status_line":       statusLine,
		"conversations":     stats.ConversationsProcessed,
		"messages_imported": stats.MessagesImported,
		"messages_created":  stats.MessagesCreated,
		"messages_updated":  stats.MessagesUpdated,
		"errors":            stats.Errors,
	})
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

func (h *ImporterHandler) InstagramStream(w http.ResponseWriter, r *http.Request) {
	instagramJob.ServeSSE(w, r)
}
func (h *ImporterHandler) InstagramCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, instagramJob.Cancel())
}
func (h *ImporterHandler) InstagramStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, instagramJob.Status())
}

// ── Facebook Messenger (standalone) ───────────────────────────────────────────

// func (h *ImporterHandler) FacebookMessengerStart(w http.ResponseWriter, r *http.Request) {
// 	if err := facebookMessengerJob.AssertNotRunning(); err != nil {
// 		writeError(w, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	var req struct {
// 		DirectoryPath string  `json:"directory_path"`
// 		UserName      *string `json:"user_name"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		writeError(w, http.StatusBadRequest, "invalid JSON body")
// 		return
// 	}
// 	if !dirExists(req.DirectoryPath) {
// 		writeError(w, http.StatusBadRequest, fmt.Sprintf("directory does not exist: %s", req.DirectoryPath))
// 		return
// 	}
// 	if h.pool == nil || h.subjectConfigRepo == nil {
// 		writeError(w, http.StatusServiceUnavailable, "Facebook Messenger import not configured")
// 		return
// 	}

// 	facebookMessengerJob.Start()
// 	facebookMessengerJob.UpdateState(map[string]any{
// 		"status": "in_progress", "status_line": "Starting Facebook Messenger import...",
// 		"conversations": 0, "messages_imported": 0, "messages_created": 0,
// 		"messages_updated": 0, "attachments_found": 0, "attachments_missing": 0,
// 		"missing_attachment_filenames": []string{}, "errors": 0,
// 	})
// 	facebookMessengerJob.Broadcast("status", map[string]any{"status_line": "Starting Facebook Messenger import..."})

// 	userName := ""
// 	if req.UserName != nil && *req.UserName != "" {
// 		userName = *req.UserName
// 	}
// 	go runFacebookInProcess(h.pool, h.subjectConfigRepo, facebookMessengerJob, req.DirectoryPath, userName, "")

// 	writeJSON(w, map[string]any{"message": "Facebook Messenger import started", "directory_path": req.DirectoryPath})
// }

// func runFacebookInProcess(pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath, userName, exportRoot string) {
// 	ctx := context.Background()
// 	defer job.Finish()

// 	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

// 	progressCallback := func(stats facebookimport.ImportStats) {
// 		statusLine := ""
// 		if stats.TotalConversations > 0 {
// 			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
// 				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
// 				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
// 				stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
// 		}
// 		job.UpdateState(map[string]any{
// 			"conversations":                stats.ConversationsProcessed,
// 			"messages_imported":            stats.MessagesImported,
// 			"messages_created":             stats.MessagesCreated,
// 			"messages_updated":             stats.MessagesUpdated,
// 			"attachments_found":            stats.AttachmentsFound,
// 			"attachments_missing":          stats.AttachmentsMissing,
// 			"missing_attachment_filenames": stats.MissingAttachmentFilenames,
// 			"errors":                       stats.Errors,
// 			"status_line":                  statusLine,
// 		})
// 		job.Broadcast("progress", job.GetState())
// 	}

// 	cancelledCheck := func() bool { return job.IsCancelled() }

// 	stats, err := facebookimport.ImportFacebookFromDirectory(ctx, storage, directoryPath, progressCallback, cancelledCheck, exportRoot, userName)

// 	if job.IsCancelled() {
// 		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
// 		job.Broadcast("cancelled", job.GetState())
// 		return
// 	}
// 	if err != nil {
// 		msg := fmt.Sprintf("import error: %s", err)
// 		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
// 		job.Broadcast("error", job.GetState())
// 		return
// 	}

// 	statusLine := fmt.Sprintf("Completed: %d conversations, %d messages (%d created, %d updated), %d attachments found, %d missing",
// 		stats.ConversationsProcessed, stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
// 		stats.AttachmentsFound, stats.AttachmentsMissing)
// 	job.UpdateState(map[string]any{
// 		"status":                       "completed",
// 		"status_line":                  statusLine,
// 		"conversations":                stats.ConversationsProcessed,
// 		"messages_imported":            stats.MessagesImported,
// 		"messages_created":             stats.MessagesCreated,
// 		"messages_updated":             stats.MessagesUpdated,
// 		"attachments_found":            stats.AttachmentsFound,
// 		"attachments_missing":          stats.AttachmentsMissing,
// 		"missing_attachment_filenames": stats.MissingAttachmentFilenames,
// 		"errors":                       stats.Errors,
// 	})
// 	runThumbnailsAfterImportIfIdle(pool)
// 	job.Broadcast("completed", job.GetState())
// }

func runFacebookAllInProcess(pool *sql.DB, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath, userName string, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	// Clear all existing Facebook data before re-importing
	job.UpdateState(map[string]any{"status_line": "Clearing existing Facebook data..."})
	job.Broadcast("progress", job.GetState())
	if err := facebookallimport.ClearFacebookAllDataForUser(ctx, pool, uid); err != nil {
		msg := fmt.Sprintf("failed to clear Facebook data: %s", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	job.UpdateState(map[string]any{"status_line": "Running parallel imports (Messenger, Albums, Places, Posts)..."})
	job.Broadcast("progress", job.GetState())

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)
	exportRoot := directoryPath
	cancelledCheck := func() bool { return job.IsCancelled() }

	var wg sync.WaitGroup
	var (
		messengerStats *facebookimport.ImportStats
		albumsStats    *facebookalbumsimport.ImportStats
		placesStats    *facebookplacesimport.ImportStats
		postsStats     *facebookpostsimport.ImportStats
		messengerErr   error
		albumsErr      error
		placesErr      error
		postsErr       error
	)

	// Messenger
	wg.Add(1)
	go func() {
		defer wg.Done()
		progressCallback := func(stats facebookimport.ImportStats) {
			job.UpdateState(map[string]any{
				"conversations":     stats.ConversationsProcessed,
				"messages_imported": stats.MessagesImported,
				"messages_created":  stats.MessagesCreated,
				"messages_updated":  stats.MessagesUpdated,
				"att_found":         stats.AttachmentsFound,
				"att_missing":       stats.AttachmentsMissing,
				"messenger_errors":  stats.Errors,
			})
			job.Broadcast("progress", job.GetState())
		}
		messengerStats, messengerErr = facebookimport.ImportFacebookFromDirectory(
			ctx, storage, directoryPath, progressCallback, cancelledCheck, exportRoot, userName,
		)
	}()

	// Albums
	wg.Add(1)
	go func() {
		defer wg.Done()
		progressCallback := func(stats facebookalbumsimport.ImportStats) {
			job.UpdateState(map[string]any{
				"albums_processed":      stats.AlbumsProcessed,
				"albums_imported":       stats.AlbumsImported,
				"album_images_imported": stats.ImagesImported,
				"album_images_found":    stats.ImagesFound,
				"album_images_missing":  stats.ImagesMissing,
				"albums_errors":         stats.Errors,
			})
			job.Broadcast("progress", job.GetState())
		}
		albumsStats, albumsErr = facebookalbumsimport.ImportFacebookAlbumsFromDirectory(
			ctx, pool, directoryPath, progressCallback, cancelledCheck, exportRoot,
		)
	}()

	// Places
	wg.Add(1)
	go func() {
		defer wg.Done()
		progressCallback := func(stats facebookplacesimport.ImportStats) {
			job.UpdateState(map[string]any{
				"places_imported": stats.PlacesImported,
				"places_created":  stats.PlacesCreated,
				"places_updated":  stats.PlacesUpdated,
			})
			job.Broadcast("progress", job.GetState())
		}
		placesStats, placesErr = facebookplacesimport.ImportFacebookPlacesFromDirectory(
			ctx, pool, directoryPath, progressCallback, cancelledCheck,
		)
	}()

	// Posts
	wg.Add(1)
	go func() {
		defer wg.Done()
		progressCallback := func(stats facebookpostsimport.ImportStats) {
			job.UpdateState(map[string]any{
				"posts_processed": stats.PostsProcessed,
				"posts_imported":  stats.PostsImported,
				"posts_updated":   stats.PostsUpdated,
				"with_media":      stats.WithMedia,
				"images_imported": stats.ImagesImported,
				"images_found":    stats.ImagesFound,
				"images_missing":  stats.ImagesMissing,
				"posts_errors":    stats.Errors,
			})
			job.Broadcast("progress", job.GetState())
		}
		postsStats, postsErr = facebookpostsimport.ImportFacebookPostsFromPath(
			ctx, pool, directoryPath, exportRoot, progressCallback, cancelledCheck,
		)
	}()

	wg.Wait()

	if job.IsCancelled() {
		job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Import cancelled."})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	// Build final stats
	stats := map[string]any{
		"status": "completed", "status_line": "Import completed",
		"conversations": 0, "messages_imported": 0, "messages_created": 0,
		"messages_updated": 0, "att_found": 0, "att_missing": 0, "messenger_errors": 0,
		"albums_processed": 0, "albums_imported": 0, "album_images_imported": 0,
		"album_images_found": 0, "album_images_missing": 0, "albums_errors": 0,
		"places_imported": 0, "places_created": 0, "places_updated": 0,
		"posts_processed": 0, "posts_imported": 0, "posts_updated": 0,
		"with_media": 0, "images_imported": 0, "images_found": 0,
		"images_missing": 0, "posts_errors": 0,
	}
	if messengerStats != nil {
		stats["conversations"] = messengerStats.ConversationsProcessed
		stats["messages_imported"] = messengerStats.MessagesImported
		stats["messages_created"] = messengerStats.MessagesCreated
		stats["messages_updated"] = messengerStats.MessagesUpdated
		stats["att_found"] = messengerStats.AttachmentsFound
		stats["att_missing"] = messengerStats.AttachmentsMissing
		stats["messenger_errors"] = messengerStats.Errors
	}
	if albumsStats != nil {
		stats["albums_processed"] = albumsStats.AlbumsProcessed
		stats["albums_imported"] = albumsStats.AlbumsImported
		stats["album_images_imported"] = albumsStats.ImagesImported
		stats["album_images_found"] = albumsStats.ImagesFound
		stats["album_images_missing"] = albumsStats.ImagesMissing
		stats["albums_errors"] = albumsStats.Errors
	}
	if placesStats != nil {
		stats["places_imported"] = placesStats.PlacesImported
		stats["places_created"] = placesStats.PlacesCreated
		stats["places_updated"] = placesStats.PlacesUpdated
	}
	if postsStats != nil {
		stats["posts_processed"] = postsStats.PostsProcessed
		stats["posts_imported"] = postsStats.PostsImported
		stats["posts_updated"] = postsStats.PostsUpdated
		stats["with_media"] = postsStats.WithMedia
		stats["images_imported"] = postsStats.ImagesImported
		stats["images_found"] = postsStats.ImagesFound
		stats["images_missing"] = postsStats.ImagesMissing
		stats["posts_errors"] = postsStats.Errors
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
		stats["status"] = "completed"
		stats["status_line"] = "Import completed with errors: " + strings.Join(errMsgs, "; ")
		stats["error_message"] = strings.Join(errMsgs, "; ")
	}

	job.UpdateState(stats)
	runThumbnailsAfterImportIfIdle(pool)
	job.Broadcast("completed", job.GetState())
}

// func (h *ImporterHandler) FacebookMessengerStream(w http.ResponseWriter, r *http.Request) {
// 	facebookMessengerJob.ServeSSE(w, r)
// }
// func (h *ImporterHandler) FacebookMessengerCancel(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookMessengerJob.Cancel())
// }
// func (h *ImporterHandler) FacebookMessengerStatus(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, facebookMessengerJob.Status())
// }

// ── Email (Gmail) process — stub ──────────────────────────────────────────────
// Gmail import uses OAuth and is not implemented in Go.
// Use POST /imap/process for IMAP-based email import instead.

func (h *ImporterHandler) EmailProcessStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeError(w, http.StatusNotImplemented,
		"Gmail import via OAuth is not implemented in the Go server. Use POST /imap/process for IMAP-based email import.")
}
func (h *ImporterHandler) EmailProcessStream(w http.ResponseWriter, r *http.Request) {
	emailProcessJob.ServeSSE(w, r)
}
func (h *ImporterHandler) EmailProcessCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, emailProcessJob.Cancel())
}
func (h *ImporterHandler) EmailProcessStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, emailProcessJob.Status())
}

func (h *ImporterHandler) EmailEmbeddingBackfillStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := emailEmbeddingBackfillJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "email embedding backfill not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}

	emailEmbeddingBackfillJob.Start()
	emailEmbeddingBackfillJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting email embedding backfill...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "error_message": nil,
	})
	emailEmbeddingBackfillJob.Broadcast("status", map[string]any{"status_line": "Starting email embedding backfill..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runEmailEmbeddingBackfill(h.pool, h.embeddingSvc, emailEmbeddingBackfillJob, uid)

	writeJSON(w, map[string]any{"message": "Email embedding backfill started", "status": "started"})
}

func runEmailEmbeddingBackfill(pool *sql.DB, embeddingSvc *service.EmbeddingService, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	missingEmbedQuery := `
		SELECT id, COALESCE(to_addresses, ''), COALESCE(from_address, ''), COALESCE(plain_text, '')
		FROM emails
		WHERE embedding_vector IS NULL
		  AND user_deleted = FALSE
		  AND COALESCE(user_id, 0) = ?1
		ORDER BY id ASC`
	if sqlutil.IsSQLite(ctx, pool) {
		missingEmbedQuery = `
		SELECT id, COALESCE(to_addresses, ''), COALESCE(from_address, ''), COALESCE(plain_text, '')
		FROM emails
		WHERE user_deleted = FALSE
		  AND COALESCE(user_id, 0) = ?1
		  AND NOT EXISTS (SELECT 1 FROM email_embeddings ee WHERE ee.rowid = emails.id)
		ORDER BY id ASC`
	}

	rows, err := pool.QueryContext(ctx, missingEmbedQuery, uid)
	if err != nil {
		msg := fmt.Sprintf("failed to query emails: %v", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}
	defer rows.Close()

	type candidate struct {
		id          int64
		toAddresses string
		fromAddress string
		plainText   string
	}
	candidates := make([]candidate, 0, 256)
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.toAddresses, &c.fromAddress, &c.plainText); err != nil {
			msg := fmt.Sprintf("failed to scan email row: %v", err)
			job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
			job.Broadcast("error", job.GetState())
			return
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		msg := fmt.Sprintf("email query cursor failed: %v", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	total := len(candidates)
	job.UpdateState(map[string]any{
		"total":       total,
		"status_line": fmt.Sprintf("Found %d emails with missing embedding vectors", total),
	})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, errorsCount := 0, 0, 0
	for i, c := range candidates {
		if job.IsCancelled() {
			job.UpdateState(map[string]any{
				"status":      "cancelled",
				"status_line": "Email embedding backfill cancelled.",
				"processed":   i,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}

		payload := buildEmailEmbeddingInput(c.toAddresses, c.fromAddress, c.plainText, 6000)
		if payload == "" {
			skipped++
			job.UpdateState(map[string]any{
				"processed":   i + 1,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d emails (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		var (
			vec     []float32
			err     error
			lastCap = 0
		)
		for _, capChars := range []int{6000, 3000, 1500, 900} {
			candidate := buildEmailEmbeddingInput(c.toAddresses, c.fromAddress, c.plainText, capChars)
			if candidate == "" {
				break
			}
			lastCap = capChars
			vec, err = embeddingSvc.EmbedText(ctx, candidate)
			if err == nil {
				break
			}
			if !strings.Contains(strings.ToLower(err.Error()), "too large") {
				break
			}
		}
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Embedding failed for email %d (max payload chars tried: %d): %v", c.id, lastCap, err),
				"error_message": fmt.Sprintf("Embedding failed for email %d (max payload chars tried: %d): %v", c.id, lastCap, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		if sqlutil.IsSQLite(ctx, pool) {
			vecBlob, serErr := sqlite_vec.SerializeFloat32(vec)
			if serErr != nil {
				errorsCount++
				job.UpdateState(map[string]any{
					"processed":     i + 1,
					"embedded":      embedded,
					"skipped":       skipped,
					"errors":        errorsCount,
					"status_line":   fmt.Sprintf("Failed to serialize embedding for email %d: %v", c.id, serErr),
					"error_message": fmt.Sprintf("Failed to serialize embedding for email %d: %v", c.id, serErr),
				})
				job.Broadcast("progress", job.GetState())
				continue
			}
			intIDsJSON, marshalErr := json.Marshal([]int64{c.id})
			if marshalErr != nil {
				errorsCount++
				job.UpdateState(map[string]any{
					"processed":     i + 1,
					"embedded":      embedded,
					"skipped":       skipped,
					"errors":        errorsCount,
					"status_line":   fmt.Sprintf("Failed to serialize email id for email %d: %v", c.id, marshalErr),
					"error_message": fmt.Sprintf("Failed to serialize email id for email %d: %v", c.id, marshalErr),
				})
				job.Broadcast("progress", job.GetState())
				continue
			}
			if err := sqlutil.Vec0Upsert(ctx, pool, "email_embeddings", c.id, vecBlob, string(intIDsJSON)); err != nil {
				errorsCount++
				job.UpdateState(map[string]any{
					"processed":     i + 1,
					"embedded":      embedded,
					"skipped":       skipped,
					"errors":        errorsCount,
					"status_line":   fmt.Sprintf("Failed saving embedding for email %d: %v", c.id, err),
					"error_message": fmt.Sprintf("Failed saving embedding for email %d: %v", c.id, err),
				})
				job.Broadcast("progress", job.GetState())
				continue
			}
		} else {
			vectorLiteral := float32SliceToVectorLiteral(vec)
			emailEmbedSQL := `UPDATE emails SET embedding_vector = $1::vector, updated_at = CURRENT_TIMESTAMP WHERE id = $2 AND COALESCE(user_id, 0) = $3`
			if _, err := pool.ExecContext(ctx, emailEmbedSQL, vectorLiteral, c.id, uid); err != nil {
				errorsCount++
				job.UpdateState(map[string]any{
					"processed":     i + 1,
					"embedded":      embedded,
					"skipped":       skipped,
					"errors":        errorsCount,
					"status_line":   fmt.Sprintf("Failed updating embedding for email %d: %v", c.id, err),
					"error_message": fmt.Sprintf("Failed updating embedding for email %d: %v", c.id, err),
				})
				job.Broadcast("progress", job.GetState())
				continue
			}
		}

		embedded++
		job.UpdateState(map[string]any{
			"processed":   i + 1,
			"embedded":    embedded,
			"skipped":     skipped,
			"errors":      errorsCount,
			"status_line": fmt.Sprintf("Processed %d/%d emails (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
		})
		job.Broadcast("progress", job.GetState())
	}

	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Email embedding backfill complete: %d embedded, %d skipped, %d errors", embedded, skipped, errorsCount),
		"processed":   total,
		"embedded":    embedded,
		"skipped":     skipped,
		"errors":      errorsCount,
	})
	job.Broadcast("completed", job.GetState())
}

func float32SliceToVectorLiteral(values []float32) string {
	if len(values) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(values) * 10)
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func buildEmailEmbeddingInput(toAddresses, fromAddress, plainText string, maxChars int) string {
	toAddresses = strings.TrimSpace(toAddresses)
	fromAddress = strings.TrimSpace(fromAddress)
	plainText = strings.TrimSpace(plainText)
	if maxChars <= 0 {
		maxChars = 900
	}

	parts := make([]string, 0, 3)
	if toAddresses != "" {
		parts = append(parts, "To: "+toAddresses)
	}
	if fromAddress != "" {
		parts = append(parts, "From: "+fromAddress)
	}
	if plainText != "" {
		parts = append(parts, "Body: "+plainText)
	}
	payload := strings.TrimSpace(strings.Join(parts, "\n"))
	if payload == "" {
		return ""
	}
	if len(payload) <= maxChars {
		return payload
	}

	// Keep address context; trim body first.
	prefixParts := make([]string, 0, 2)
	if toAddresses != "" {
		prefixParts = append(prefixParts, "To: "+toAddresses)
	}
	if fromAddress != "" {
		prefixParts = append(prefixParts, "From: "+fromAddress)
	}
	prefix := strings.TrimSpace(strings.Join(prefixParts, "\n"))
	if prefix == "" {
		if maxChars > len(payload) {
			return payload
		}
		return strings.TrimSpace(payload[:maxChars])
	}

	remaining := maxChars - len(prefix) - 1 // newline separator
	if remaining <= 0 {
		if maxChars > len(prefix) {
			return prefix
		}
		return strings.TrimSpace(prefix[:maxChars])
	}
	body := plainText
	if len(body) > remaining {
		body = body[:remaining]
	}
	return strings.TrimSpace(prefix + "\n" + body)
}

func (h *ImporterHandler) EmailEmbeddingBackfillStream(w http.ResponseWriter, r *http.Request) {
	emailEmbeddingBackfillJob.ServeSSE(w, r)
}

func (h *ImporterHandler) EmailEmbeddingBackfillCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, emailEmbeddingBackfillJob.Cancel())
}

func (h *ImporterHandler) EmailEmbeddingBackfillStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, emailEmbeddingBackfillJob.Status())
}

// func (h *ImporterHandler) MessageEmbeddingBackfillStart(w http.ResponseWriter, r *http.Request) {
// 	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
// 		return
// 	}
// 	if err := messageEmbeddingBackfillJob.AssertNotRunning(); err != nil {
// 		writeError(w, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	if h.pool == nil {
// 		writeError(w, http.StatusServiceUnavailable, "message embedding backfill not configured")
// 		return
// 	}
// 	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
// 		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
// 		return
// 	}

// 	messageEmbeddingBackfillJob.Start()
// 	messageEmbeddingBackfillJob.UpdateState(map[string]any{
// 		"status": "in_progress", "status_line": "Starting message embedding backfill...",
// 		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "error_message": nil,
// 	})
// 	messageEmbeddingBackfillJob.Broadcast("status", map[string]any{"status_line": "Starting message embedding backfill..."})

// 	uid := appctx.UserIDFromCtx(r.Context())
// 	go runMessageEmbeddingBackfill(h.pool, h.embeddingSvc, messageEmbeddingBackfillJob, uid)

// 	writeJSON(w, map[string]any{"message": "Message embedding backfill started", "status": "started"})
// }

func runMessageEmbeddingBackfill(pool *sql.DB, embeddingSvc *service.EmbeddingService, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	rows, err := pool.QueryContext(ctx, `
		SELECT id, COALESCE(chat_session, ''), COALESCE(text, '')
		FROM messages
		WHERE embedding_vector IS NULL
		  AND COALESCE(user_id, 0) = ?1
		  AND text IS NOT NULL
		ORDER BY id ASC
	`, uid)
	if err != nil {
		msg := fmt.Sprintf("failed to query messages: %v", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}
	defer rows.Close()

	type candidate struct {
		id          int64
		chatSession string
		text        string
	}
	candidates := make([]candidate, 0, 256)
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.chatSession, &c.text); err != nil {
			msg := fmt.Sprintf("failed to scan message row: %v", err)
			job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
			job.Broadcast("error", job.GetState())
			return
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		msg := fmt.Sprintf("message query cursor failed: %v", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	total := len(candidates)
	job.UpdateState(map[string]any{
		"total":       total,
		"status_line": fmt.Sprintf("Found %d messages with missing embedding vectors", total),
	})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, errorsCount := 0, 0, 0
	for i, c := range candidates {
		if job.IsCancelled() {
			job.UpdateState(map[string]any{
				"status":      "cancelled",
				"status_line": "Message embedding backfill cancelled.",
				"processed":   i,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}

		var (
			vec     []float32
			err     error
			lastCap = 0
		)
		for _, capChars := range []int{6000, 3000, 1500, 900} {
			candidate := buildMessageEmbeddingInput(c.chatSession, c.text, capChars)
			if candidate == "" {
				break
			}
			lastCap = capChars
			vec, err = embeddingSvc.EmbedText(ctx, candidate)
			if err == nil {
				break
			}
			if !strings.Contains(strings.ToLower(err.Error()), "too large") {
				break
			}
		}
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Embedding failed for message %d (max payload chars tried: %d): %v", c.id, lastCap, err),
				"error_message": fmt.Sprintf("Embedding failed for message %d (max payload chars tried: %d): %v", c.id, lastCap, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		if vec == nil {
			skipped++
			job.UpdateState(map[string]any{
				"processed":   i + 1,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		vectorLiteral := float32SliceToVectorLiteral(vec)
		msgEmbedSQL := `UPDATE messages SET embedding_vector = $1::vector, updated_at = CURRENT_TIMESTAMP WHERE id = $2 AND COALESCE(user_id, 0) = $3`
		if sqlutil.IsSQLite(ctx, pool) {
			msgEmbedSQL = `UPDATE messages SET embedding_vector = ?1, updated_at = CURRENT_TIMESTAMP WHERE id = ?2 AND COALESCE(user_id, 0) = ?3`
		}
		if _, err := pool.ExecContext(ctx, msgEmbedSQL, vectorLiteral, c.id, uid); err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Failed updating embedding for message %d: %v", c.id, err),
				"error_message": fmt.Sprintf("Failed updating embedding for message %d: %v", c.id, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		embedded++
		job.UpdateState(map[string]any{
			"processed":   i + 1,
			"embedded":    embedded,
			"skipped":     skipped,
			"errors":      errorsCount,
			"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
		})
		job.Broadcast("progress", job.GetState())
	}

	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Message embedding backfill complete: %d embedded, %d skipped, %d errors", embedded, skipped, errorsCount),
		"processed":   total,
		"embedded":    embedded,
		"skipped":     skipped,
		"errors":      errorsCount,
	})
	job.Broadcast("completed", job.GetState())
}

func buildMessageEmbeddingInput(chatSession, text string, maxChars int) string {
	chatSession = strings.TrimSpace(chatSession)
	text = strings.TrimSpace(text)
	if maxChars <= 0 {
		maxChars = 900
	}
	parts := make([]string, 0, 2)
	if chatSession != "" {
		parts = append(parts, "Chat Session: "+chatSession)
	}
	if text != "" {
		parts = append(parts, "Text: "+text)
	}
	payload := strings.TrimSpace(strings.Join(parts, "\n"))
	if payload == "" {
		return ""
	}
	if len(payload) <= maxChars {
		return payload
	}

	prefix := ""
	if chatSession != "" {
		prefix = "Chat Session: " + chatSession
	}
	if prefix == "" {
		return strings.TrimSpace(payload[:maxChars])
	}
	remaining := maxChars - len(prefix) - 1
	if remaining <= 0 {
		if maxChars > len(prefix) {
			return prefix
		}
		return strings.TrimSpace(prefix[:maxChars])
	}
	body := text
	if len(body) > remaining {
		body = body[:remaining]
	}
	return strings.TrimSpace(prefix + "\n" + body)
}

// func (h *ImporterHandler) MessageEmbeddingBackfillStream(w http.ResponseWriter, r *http.Request) {
// 	messageEmbeddingBackfillJob.ServeSSE(w, r)
// }

// func (h *ImporterHandler) MessageEmbeddingBackfillCancel(w http.ResponseWriter, r *http.Request) {
// 	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
// 		return
// 	}
// 	writeJSON(w, messageEmbeddingBackfillJob.Cancel())
// }

// func (h *ImporterHandler) MessageEmbeddingBackfillStatus(w http.ResponseWriter, r *http.Request) {
// 	writeJSON(w, messageEmbeddingBackfillJob.Status())
// }

func (h *ImporterHandler) MessageContextEmbeddingBackfillStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	if err := messageContextEmbeddingBackfillJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "message context embedding backfill not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}

	messageContextEmbeddingBackfillJob.Start()
	messageContextEmbeddingBackfillJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting message context embedding backfill...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "error_message": nil,
	})
	messageContextEmbeddingBackfillJob.Broadcast("status", map[string]any{"status_line": "Starting message context embedding backfill..."})

	uid := appctx.UserIDFromCtx(r.Context())
	go runMessageContextEmbeddingBackfill(h.pool, h.embeddingSvc, messageContextEmbeddingBackfillJob, uid)

	writeJSON(w, map[string]any{"message": "Message context embedding backfill started", "status": "started"})
}

type messageContextCandidate struct {
	id          int64
	chatSession string
	text        string
}

func runMessageContextEmbeddingBackfill(pool *sql.DB, embeddingSvc *service.EmbeddingService, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	const (
		messageEmbeddingModel   = "embeddinggemma:latest"
		messageEmbeddingBack    = 5
		messageEmbeddingForward = 5
	)

	rows, err := pool.QueryContext(ctx, `
		SELECT m.id, COALESCE(m.chat_session, ''), COALESCE(m.text, '')
		FROM messages m
		WHERE COALESCE(m.user_id, 0) = ?
		  AND m.text IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM message_embeddings me WHERE me.rowid = m.id
		  )
		ORDER BY COALESCE(m.chat_session, ''), CASE WHEN m.message_date IS NULL THEN 1 ELSE 0 END, m.message_date, m.id
	`, uid)
	if err != nil {
		msg := fmt.Sprintf("failed to query messages for context embeddings: %v", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}
	defer rows.Close()

	all := make([]messageContextCandidate, 0, 512)
	for rows.Next() {
		var c messageContextCandidate
		if err := rows.Scan(&c.id, &c.chatSession, &c.text); err != nil {
			msg := fmt.Sprintf("failed to scan message row: %v", err)
			job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
			job.Broadcast("error", job.GetState())
			return
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		msg := fmt.Sprintf("message query cursor failed: %v", err)
		job.UpdateState(map[string]any{"status": "error", "status_line": msg, "error_message": msg})
		job.Broadcast("error", job.GetState())
		return
	}

	total := len(all)
	job.UpdateState(map[string]any{
		"total":       total,
		"status_line": fmt.Sprintf("Found %d messages for context embedding windows", total),
	})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, errorsCount := 0, 0, 0
	seenAnchors := make(map[int64]struct{}, total)
	for i := range all {
		if job.IsCancelled() {
			job.UpdateState(map[string]any{
				"status":      "cancelled",
				"status_line": "Message context embedding backfill cancelled.",
				"processed":   i,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		// Skip every second record to reduce embedding workload.
		// if i%2 == 1 {
		// 	skipped++
		// 	job.UpdateState(map[string]any{
		// 		"processed":   i + 1,
		// 		"embedded":    embedded,
		// 		"skipped":     skipped,
		// 		"errors":      errorsCount,
		// 		"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
		// 	})
		// 	job.Broadcast("progress", job.GetState())
		// 	continue
		// }

		anchorID := all[i].id
		if _, ok := seenAnchors[anchorID]; ok {
			skipped++
			job.UpdateState(map[string]any{
				"processed":   i + 1,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		seenAnchors[anchorID] = struct{}{}

		windowIDs, contextPayload := buildMessageContextWindow(all, i, messageEmbeddingBack, messageEmbeddingForward)
		if len(windowIDs) == 0 || strings.TrimSpace(contextPayload) == "" {
			skipped++
			job.UpdateState(map[string]any{
				"processed":   i + 1,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		payloadHash := hashMessageEmbeddingPayload(contextPayload, windowIDs)
		metaMatch, err := isMessageEmbeddingMetaCurrent(ctx, pool, anchorID, messageEmbeddingModel, messageEmbeddingBack, messageEmbeddingForward, payloadHash)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Metadata check failed for message %d: %v", anchorID, err),
				"error_message": fmt.Sprintf("Metadata check failed for message %d: %v", anchorID, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		if metaMatch {
			skipped++
			job.UpdateState(map[string]any{
				"processed":   i + 1,
				"embedded":    embedded,
				"skipped":     skipped,
				"errors":      errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		vec, err := embeddingSvc.EmbedTextWithModel(ctx, contextPayload, messageEmbeddingModel)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Embedding failed for message %d: %v", anchorID, err),
				"error_message": fmt.Sprintf("Embedding failed for message %d: %v", anchorID, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		vecBlob, err := sqlite_vec.SerializeFloat32(vec)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Failed to serialize embedding for message %d: %v", anchorID, err),
				"error_message": fmt.Sprintf("Failed to serialize embedding for message %d: %v", anchorID, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		intIDsJSON, err := json.Marshal(windowIDs)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Failed to serialize message id window for message %d: %v", anchorID, err),
				"error_message": fmt.Sprintf("Failed to serialize message id window for message %d: %v", anchorID, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		if err := sqlutil.Vec0Upsert(ctx, pool, "message_embeddings", anchorID, vecBlob, string(intIDsJSON)); err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Failed saving context embedding for message %d: %v", anchorID, err),
				"error_message": fmt.Sprintf("Failed saving context embedding for message %d: %v", anchorID, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		if err := upsertMessageEmbeddingMeta(ctx, pool, anchorID, messageEmbeddingModel, messageEmbeddingBack, messageEmbeddingForward, payloadHash); err != nil {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     i + 1,
				"embedded":      embedded,
				"skipped":       skipped,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("Failed saving embedding metadata for message %d: %v", anchorID, err),
				"error_message": fmt.Sprintf("Failed saving embedding metadata for message %d: %v", anchorID, err),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}

		embedded++
		job.UpdateState(map[string]any{
			"processed":   i + 1,
			"embedded":    embedded,
			"skipped":     skipped,
			"errors":      errorsCount,
			"status_line": fmt.Sprintf("Processed %d/%d messages (%d embedded, %d skipped, %d errors)", i+1, total, embedded, skipped, errorsCount),
		})
		job.Broadcast("progress", job.GetState())
	}

	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Message context embedding backfill complete: %d embedded, %d skipped, %d errors", embedded, skipped, errorsCount),
		"processed":   total,
		"embedded":    embedded,
		"skipped":     skipped,
		"errors":      errorsCount,
	})
	job.Broadcast("completed", job.GetState())
}

func hashMessageEmbeddingPayload(contextPayload string, windowIDs []int64) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strings.TrimSpace(contextPayload)))
	_, _ = h.Write([]byte("|"))
	for i, id := range windowIDs {
		if i > 0 {
			_, _ = h.Write([]byte(","))
		}
		_, _ = h.Write([]byte(strconv.FormatInt(id, 10)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func isMessageEmbeddingMetaCurrent(ctx context.Context, pool *sql.DB, messageID int64, model string, back, forward int, payloadHash string) (bool, error) {
	var (
		storedModel   string
		storedBack    int
		storedForward int
		storedHash    string
	)
	err := pool.QueryRowContext(ctx, `
		SELECT model, window_back, window_forward, content_hash
		FROM message_embedding_meta
		WHERE message_id = ?
	`, messageID).Scan(&storedModel, &storedBack, &storedForward, &storedHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return storedModel == model && storedBack == back && storedForward == forward && storedHash == payloadHash, nil
}

func upsertMessageEmbeddingMeta(ctx context.Context, pool *sql.DB, messageID int64, model string, back, forward int, payloadHash string) error {
	_, err := pool.ExecContext(ctx, `
		INSERT INTO message_embedding_meta(message_id, model, window_back, window_forward, content_hash, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(message_id) DO UPDATE SET
			model = excluded.model,
			window_back = excluded.window_back,
			window_forward = excluded.window_forward,
			content_hash = excluded.content_hash,
			updated_at = excluded.updated_at
	`, messageID, model, back, forward, payloadHash)
	return err
}

func buildMessageContextWindow(messages []messageContextCandidate, idx, back, forward int) ([]int64, string) {
	if idx < 0 || idx >= len(messages) {
		return nil, ""
	}
	if back < 0 {
		back = 0
	}
	if forward < 0 {
		forward = 0
	}
	targetChat := messages[idx].chatSession
	start := idx
	for start > 0 && messages[start-1].chatSession == targetChat {
		start--
	}
	end := idx
	for end+1 < len(messages) && messages[end+1].chatSession == targetChat {
		end++
	}
	winStart := idx - back
	if winStart < start {
		winStart = start
	}
	winEnd := idx + forward
	if winEnd > end {
		winEnd = end
	}

	ids := make([]int64, 0, winEnd-winStart+1)
	lines := make([]string, 0, winEnd-winStart+1)
	for i := winStart; i <= winEnd; i++ {
		m := messages[i]
		ids = append(ids, m.id)
		text := strings.TrimSpace(m.text)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("Chat Session: %s\nText: %s", strings.TrimSpace(m.chatSession), text))
	}
	return ids, strings.TrimSpace(strings.Join(lines, "\n\n"))
}

func (h *ImporterHandler) MessageContextEmbeddingBackfillStream(w http.ResponseWriter, r *http.Request) {
	messageContextEmbeddingBackfillJob.ServeSSE(w, r)
}

func (h *ImporterHandler) MessageContextEmbeddingBackfillCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	writeJSON(w, messageContextEmbeddingBackfillJob.Cancel())
}

func (h *ImporterHandler) MessageContextEmbeddingBackfillStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, messageContextEmbeddingBackfillJob.Status())
}

// ── stdout parsers ────────────────────────────────────────────────────────────

var reInt = regexp.MustCompile(`\d+`)

// // parseMessageStdout parses the shared stdout format used by whatsapp, imessage,
// // and instagram import commands. Pass includeAttachments=true for whatsapp/imessage.
// func parseMessageStdout(s string, includeAttachments bool) map[string]any {
// 	reConvs := regexp.MustCompile(`Processed (\d+) conversation`)
// 	reImported := regexp.MustCompile(`Imported (\d+) message.*\((\d+) created, (\d+) updated\)`)
// 	reAttach := regexp.MustCompile(`Found (\d+) attachment.*?, (\d+) missing`)
// 	reSkipped := regexp.MustCompile(`Skipped invalid messages.*?:\s*(\d+)`)

// 	stats := map[string]any{
// 		"conversations":     0,
// 		"messages_imported": 0,
// 		"messages_created":  0,
// 		"messages_updated":  0,
// 		"errors":            0,
// 	}
// 	if includeAttachments {
// 		stats["attachments_found"] = 0
// 		stats["attachments_missing"] = 0
// 		stats["missing_attachment_filenames"] = []string{}
// 	}

// 	var missingFiles []string
// 	inMissing := false
// 	for _, line := range strings.Split(s, "\n") {
// 		if m := reConvs.FindStringSubmatch(line); len(m) > 1 {
// 			n, _ := strconv.Atoi(m[1])
// 			stats["conversations"] = n
// 		} else if m := reImported.FindStringSubmatch(line); len(m) > 3 {
// 			total, _ := strconv.Atoi(m[1])
// 			created, _ := strconv.Atoi(m[2])
// 			updated, _ := strconv.Atoi(m[3])
// 			stats["messages_imported"] = total
// 			stats["messages_created"] = created
// 			stats["messages_updated"] = updated
// 		} else if includeAttachments {
// 			if m := reAttach.FindStringSubmatch(line); len(m) > 2 {
// 				found, _ := strconv.Atoi(m[1])
// 				missing, _ := strconv.Atoi(m[2])
// 				stats["attachments_found"] = found
// 				stats["attachments_missing"] = missing
// 			}
// 			if strings.TrimSpace(line) == "Missing attachment files:" {
// 				inMissing = true
// 				continue
// 			}
// 			if inMissing && strings.HasPrefix(line, "  - ") {
// 				missingFiles = append(missingFiles, strings.TrimPrefix(line, "  - "))
// 			} else if inMissing && !strings.HasPrefix(line, "  ") {
// 				inMissing = false
// 			}
// 		}
// 		if m := reSkipped.FindStringSubmatch(line); len(m) > 1 {
// 			n, _ := strconv.Atoi(m[1])
// 			stats["errors"] = n
// 		}
// 	}
// 	if includeAttachments && missingFiles != nil {
// 		stats["missing_attachment_filenames"] = missingFiles
// 	}
// 	return stats
// }

// func parseInt(s string) int {
// 	m := reInt.FindString(s)
// 	if m == "" {
// 		return 0
// 	}
// 	n, _ := strconv.Atoi(m)
// 	return n
// }

// func parseFacebookAlbumsStdout(s string) (albumsProcessed, albumsImported, imagesImported, imagesFound, imagesMissing, errs int, missing []string, msg string) {
// 	reAlbums := regexp.MustCompile(`Processed (\d+) album\(s\)`)
// 	reAlbumsImported := regexp.MustCompile(`Albums imported: (\d+)`)
// 	reImages := regexp.MustCompile(`Images imported: (\d+) \(found: (\d+), missing: (\d+)\)`)
// 	reErrors := regexp.MustCompile(`Errors: (\d+)`)

// 	if m := reAlbums.FindStringSubmatch(s); len(m) > 1 {
// 		albumsProcessed, _ = strconv.Atoi(m[1])
// 	}
// 	if m := reAlbumsImported.FindStringSubmatch(s); len(m) > 1 {
// 		albumsImported, _ = strconv.Atoi(m[1])
// 	}
// 	if m := reImages.FindStringSubmatch(s); len(m) > 3 {
// 		imagesImported, _ = strconv.Atoi(m[1])
// 		imagesFound, _ = strconv.Atoi(m[2])
// 		imagesMissing, _ = strconv.Atoi(m[3])
// 	}
// 	if m := reErrors.FindStringSubmatch(s); len(m) > 1 {
// 		errs, _ = strconv.Atoi(m[1])
// 	}
// 	inMissing := false
// 	for _, line := range strings.Split(s, "\n") {
// 		if strings.TrimSpace(line) == "Missing image files:" {
// 			inMissing = true
// 			continue
// 		}
// 		if inMissing && strings.HasPrefix(line, "  - ") {
// 			missing = append(missing, strings.TrimPrefix(line, "  - "))
// 		}
// 	}

// 	parts := []string{"Import completed"}
// 	if albumsProcessed > 0 {
// 		parts = append(parts, fmt.Sprintf("Processed %d album(s)", albumsProcessed))
// 	}
// 	if albumsImported > 0 {
// 		parts = append(parts, fmt.Sprintf("Imported %d album(s) with %d image(s)", albumsImported, imagesImported))
// 	}
// 	msg = strings.Join(parts, ". ")
// 	return
// }

// func parseFacebookPostsStdout(s string) map[string]any {
// 	stats := map[string]any{}
// 	for _, line := range strings.Split(s, "\n") {
// 		if strings.HasPrefix(line, "POSTS_COMPLETE: ") {
// 			for _, part := range strings.Fields(line[len("POSTS_COMPLETE: "):]) {
// 				kv := strings.SplitN(part, "=", 2)
// 				if len(kv) == 2 {
// 					n, _ := strconv.Atoi(kv[1])
// 					switch kv[0] {
// 					case "posts":
// 						stats["posts_processed"] = n
// 					case "new":
// 						stats["posts_imported"] = n
// 					case "updated":
// 						stats["posts_updated"] = n
// 					case "with_media":
// 						stats["with_media"] = n
// 					case "images":
// 						stats["images_imported"] = n
// 					case "found":
// 						stats["images_found"] = n
// 					case "missing":
// 						stats["images_missing"] = n
// 					case "errors":
// 						stats["errors"] = n
// 					}
// 				}
// 			}
// 		}
// 	}
// 	return stats
// }

// func parseFacebookPlacesStdout(s string) map[string]any {
// 	stats := map[string]any{}
// 	for _, line := range strings.Split(s, "\n") {
// 		if strings.HasPrefix(line, "PLACES_COMPLETE: ") {
// 			for _, part := range strings.Fields(line[len("PLACES_COMPLETE: "):]) {
// 				kv := strings.SplitN(part, "=", 2)
// 				if len(kv) == 2 {
// 					n, _ := strconv.Atoi(kv[1])
// 					switch kv[0] {
// 					case "places":
// 						stats["places_imported"] = n
// 					case "created":
// 						stats["places_created"] = n
// 					case "updated":
// 						stats["places_updated"] = n
// 					}
// 				}
// 			}
// 		}
// 	}
// 	return stats
// }

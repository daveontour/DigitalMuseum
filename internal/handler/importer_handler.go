package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"strings"
	"sync"

	contactsimport "github.com/daveontour/digitalmuseum/internal/import/contacts"
	facebookimport "github.com/daveontour/digitalmuseum/internal/import/facebook"
	facebookalbumsimport "github.com/daveontour/digitalmuseum/internal/import/facebookalbums"
	facebookallimport "github.com/daveontour/digitalmuseum/internal/import/facebookall"
	facebookplacesimport "github.com/daveontour/digitalmuseum/internal/import/facebookplaces"
	facebookpostsimport "github.com/daveontour/digitalmuseum/internal/import/facebookposts"
	filesystemimport "github.com/daveontour/digitalmuseum/internal/import/filesystem"
	imessageimport "github.com/daveontour/digitalmuseum/internal/import/imessage"
	instagramimport "github.com/daveontour/digitalmuseum/internal/import/instagram"
	thumbnailsimport "github.com/daveontour/digitalmuseum/internal/import/thumbnails"
	whatsappimport "github.com/daveontour/digitalmuseum/internal/import/whatsapp"
	"github.com/daveontour/digitalmuseum/internal/importer"
	"github.com/daveontour/digitalmuseum/internal/importstorage"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		"errors": 0, "error_messages": []string{},
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

	imageExportJob = importer.NewImportJob("Image export", map[string]any{
		"status": "idle", "status_line": nil, "total": 0, "processed": 0,
		"exported": 0, "skipped": 0, "errors": 0, "error_message": nil, "error_messages": []string{},
	})
)

// ── ImporterHandler ───────────────────────────────────────────────────────────

// ImporterHandler handles all import job HTTP endpoints.
type ImporterHandler struct {
	excludePatterns   []string
	imageRepo         *repository.ImageRepo
	pool              *pgxpool.Pool
	subjectConfigRepo *repository.SubjectConfigRepo
}

// ImporterHandlerDeps holds dependencies for NewImporterHandler.
type ImporterHandlerDeps struct {
	ExcludePatterns   []string
	ImageRepo         *repository.ImageRepo
	Pool              *pgxpool.Pool
	SubjectConfigRepo *repository.SubjectConfigRepo
}

// NewImporterHandler creates an ImporterHandler.
func NewImporterHandler(deps ImporterHandlerDeps) *ImporterHandler {
	return &ImporterHandler{
		excludePatterns:   deps.ExcludePatterns,
		imageRepo:         deps.ImageRepo,
		pool:              deps.Pool,
		subjectConfigRepo: deps.SubjectConfigRepo,
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

	// Image export (export images to filesystem)
	r.Post("/images/export", h.ImageExportStart)
	r.Get("/images/export/stream", h.ImageExportStream)
	r.Post("/images/export/cancel", h.ImageExportCancel)
	r.Get("/images/export/status", h.ImageExportStatus)

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
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// runThumbnailsAfterImportIfIdle starts thumbnail processing if the thumbnails job is idle.
// Called after image-loading imports (filesystem, etc.) and email imports (IMAP/Gmail) complete successfully.
func runThumbnailsAfterImportIfIdle(pool *pgxpool.Pool) {
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
	go runThumbnailsInProcess(pool, thumbnailsJob, false)
}

// ── Filesystem ────────────────────────────────────────────────────────────────

func (h *ImporterHandler) FilesystemStart(w http.ResponseWriter, r *http.Request) {
	if err := filesystemJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		RootDirectory string `json:"root_directory"`
		MaxImages     *int   `json:"max_images"`
		ReferenceMode bool   `json:"reference_mode"`
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
		"errors": 0, "error_messages": []string{},
	})
	filesystemJob.Broadcast("status", map[string]any{"status_line": "Starting filesystem images import..."})

	maxImages := req.MaxImages
	if maxImages != nil && *maxImages <= 0 {
		maxImages = nil
	}
	go runFilesystemInProcess(h.pool, filesystemJob, validPaths, h.excludePatterns, maxImages, req.ReferenceMode)

	writeJSON(w, map[string]any{
		"message":        "Filesystem images import started",
		"root_directory": req.RootDirectory,
	})
}

func runFilesystemInProcess(pool *pgxpool.Pool, job *importer.ImportJob, directories []string, excludePatterns []string, maxImages *int, referenceMode bool) {
	ctx := context.Background()
	defer job.Finish()

	storage := importstorage.NewImageStorage(pool)

	progressCallback := func(stats filesystemimport.ImportStats) {
		statusLine := fmt.Sprintf("Processing file %d of %d: %s | Imported: %d, Referenced: %d, Updated: %d, Errors: %d",
			stats.FilesProcessed, stats.TotalFiles, stats.CurrentFile,
			stats.ImagesImported, stats.ImagesReferenced, stats.ImagesUpdated, stats.Errors)
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
		job.Broadcast("progress", job.GetState())
	}

	cancelledCheck := func() bool { return job.IsCancelled() }

	stats, err := filesystemimport.ImportImagesFromDirectories(ctx, storage, directories, excludePatterns, maxImages, referenceMode, progressCallback, cancelledCheck)

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
	writeJSON(w, filesystemJob.Cancel())
}
func (h *ImporterHandler) FilesystemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, filesystemJob.Status())
}

// ── Thumbnails ────────────────────────────────────────────────────────────────

func (h *ImporterHandler) ThumbnailsStart(w http.ResponseWriter, r *http.Request) {
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

	go runThumbnailsInProcess(h.pool, thumbnailsJob, reprocess)

	writeJSON(w, map[string]any{"message": "Thumbnail processing started", "status": "started"})
}

func (h *ImporterHandler) ThumbnailsStartAsync(w http.ResponseWriter, r *http.Request) {
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

	go runThumbnailsInProcess(h.pool, thumbnailsJob, reprocess)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "accepted"})
}

func runThumbnailsInProcess(pool *pgxpool.Pool, job *importer.ImportJob, reprocess bool) {
	ctx := context.Background()
	defer job.Finish()

	// Region updates before (match Python behavior)
	_, _ = pool.Exec(ctx, "SELECT update_location_regions()")
	_, _ = pool.Exec(ctx, "SELECT update_image_location_regions()")

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

	// Region updates after (match Python behavior)
	_, _ = pool.Exec(ctx, "SELECT update_location_regions()")
	_, _ = pool.Exec(ctx, "SELECT update_image_location_regions()")

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
	writeJSON(w, thumbnailsJob.Cancel())
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
	go runFacebookAllInProcess(h.pool, h.subjectConfigRepo, facebookAllJob, req.DirectoryPath, userName)

	writeJSON(w, map[string]any{"message": "Facebook All import started", "directory_path": req.DirectoryPath})
}

func (h *ImporterHandler) FacebookAllStream(w http.ResponseWriter, r *http.Request) {
	facebookAllJob.ServeSSE(w, r)
}
func (h *ImporterHandler) FacebookAllCancel(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, facebookAllJob.Cancel())
}
func (h *ImporterHandler) FacebookAllStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, facebookAllJob.Status())
}

// ── Contacts extract ──────────────────────────────────────────────────────────

func (h *ImporterHandler) ContactsExtractStart(w http.ResponseWriter, r *http.Request) {
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

	go runContactsExtractInProcess(h.pool, contactsExtractJob)

	writeJSON(w, map[string]any{"message": "Contacts extract started", "status": "started"})
}

func runContactsExtractInProcess(pool *pgxpool.Pool, job *importer.ImportJob) {
	ctx := context.Background()
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
	writeJSON(w, contactsExtractJob.Cancel())
}
func (h *ImporterHandler) ContactsExtractStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, contactsExtractJob.Status())
}

// ── Reference import ──────────────────────────────────────────────────────────

func (h *ImporterHandler) ReferenceImportStart(w http.ResponseWriter, r *http.Request) {
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

	go runReferenceImport(h.imageRepo, referenceImportJob, h.pool)

	writeJSON(w, map[string]any{"message": "Reference import started", "status": "started"})
}

func runReferenceImport(repo *repository.ImageRepo, job *importer.ImportJob, pool *pgxpool.Pool) {
	ctx := context.Background()
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
	writeJSON(w, referenceImportJob.Cancel())
}
func (h *ImporterHandler) ReferenceImportStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, referenceImportJob.Status())
}

// ── Image export ───────────────────────────────────────────────────────────────

func (h *ImporterHandler) ImageExportStart(w http.ResponseWriter, r *http.Request) {
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

	go runImageExport(h.imageRepo, imageExportJob, targetDir)

	writeJSON(w, map[string]any{"message": "Image export started", "status": "started"})
}

func runImageExport(repo *repository.ImageRepo, job *importer.ImportJob, targetDir string) {
	ctx := context.Background()
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
	writeJSON(w, imageExportJob.Cancel())
}
func (h *ImporterHandler) ImageExportStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imageExportJob.Status())
}

// ── WhatsApp ──────────────────────────────────────────────────────────────────

func (h *ImporterHandler) WhatsAppStart(w http.ResponseWriter, r *http.Request) {
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

	go runWhatsAppInProcess(h.pool, h.subjectConfigRepo, whatsappJob, req.DirectoryPath)

	writeJSON(w, map[string]any{"message": "WhatsApp import started", "directory_path": req.DirectoryPath})
}

func runWhatsAppInProcess(pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath string) {
	ctx := context.Background()
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats whatsappimport.ImportStats) {
		statusLine := ""
		if stats.TotalConversations > 0 {
			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
				stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
		}
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
	writeJSON(w, whatsappJob.Cancel())
}
func (h *ImporterHandler) WhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, whatsappJob.Status())
}

// ── iMessage ──────────────────────────────────────────────────────────────────

func (h *ImporterHandler) IMessageStart(w http.ResponseWriter, r *http.Request) {
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

	go runIMessageInProcess(h.pool, h.subjectConfigRepo, imessageJob, req.DirectoryPath)

	writeJSON(w, map[string]any{"message": "iMessage import started", "directory_path": req.DirectoryPath})
}

func runIMessageInProcess(pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath string) {
	ctx := context.Background()
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats imessageimport.ImportStats) {
		statusLine := ""
		if stats.TotalConversations > 0 {
			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
				stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
		}
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

func (h *ImporterHandler) IMessageStream(w http.ResponseWriter, r *http.Request) {
	imessageJob.ServeSSE(w, r)
}
func (h *ImporterHandler) IMessageCancel(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imessageJob.Cancel())
}
func (h *ImporterHandler) IMessageStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imessageJob.Status())
}

// ── Instagram ─────────────────────────────────────────────────────────────────

func (h *ImporterHandler) InstagramStart(w http.ResponseWriter, r *http.Request) {
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
	go runInstagramInProcess(h.pool, h.subjectConfigRepo, instagramJob, req.DirectoryPath, userName, exportRoot)

	writeJSON(w, map[string]any{"message": "Instagram import started", "directory_path": req.DirectoryPath})
}

func runInstagramInProcess(pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath, userName, exportRoot string) {
	ctx := context.Background()
	defer job.Finish()

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)

	progressCallback := func(stats instagramimport.ImportStats) {
		statusLine := ""
		if stats.TotalConversations > 0 {
			statusLine = fmt.Sprintf("Processing conversation %d of %d: %s | Messages: %d (%d created, %d updated) | Attachments: %d found, %d missing | Errors: %d",
				stats.ConversationsProcessed, stats.TotalConversations, stats.CurrentConversation,
				stats.MessagesImported, stats.MessagesCreated, stats.MessagesUpdated,
				stats.AttachmentsFound, stats.AttachmentsMissing, stats.Errors)
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

// func runFacebookInProcess(pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath, userName, exportRoot string) {
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

func runFacebookAllInProcess(pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job *importer.ImportJob, directoryPath, userName string) {
	ctx := context.Background()
	defer job.Finish()

	// Clear all existing Facebook data before re-importing
	job.UpdateState(map[string]any{"status_line": "Clearing existing Facebook data..."})
	job.Broadcast("progress", job.GetState())
	if err := facebookallimport.ClearFacebookAllData(ctx, pool); err != nil {
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

func (h *ImporterHandler) EmailProcessStart(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented,
		"Gmail import via OAuth is not implemented in the Go server. Use POST /imap/process for IMAP-based email import.")
}
func (h *ImporterHandler) EmailProcessStream(w http.ResponseWriter, r *http.Request) {
	emailProcessJob.ServeSSE(w, r)
}
func (h *ImporterHandler) EmailProcessCancel(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, emailProcessJob.Cancel())
}
func (h *ImporterHandler) EmailProcessStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, emailProcessJob.Status())
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

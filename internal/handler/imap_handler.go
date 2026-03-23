package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	appimporter "github.com/daveontour/digitalmuseum/internal/importer"
	imapimport "github.com/daveontour/digitalmuseum/internal/import/imap"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IMAPHandler handles all /imap/* routes.
type IMAPHandler struct {
	pool *pgxpool.Pool
	job  *appimporter.ImportJob
}

// NewIMAPHandler creates an IMAPHandler.
func NewIMAPHandler(pool *pgxpool.Pool) *IMAPHandler {
	return &IMAPHandler{
		pool: pool,
		job: appimporter.NewImportJob("IMAP import", map[string]any{
			"status":               "idle",
			"status_line":          nil,
			"error_message":        nil,
			"current_folder":       nil,
			"current_folder_index": 0,
			"total_folders":        0,
			"emails_processed":     0,
			"folders":              []string{},
		}),
	}
}

// RegisterRoutes mounts all IMAP routes.
func (h *IMAPHandler) RegisterRoutes(r chi.Router) {
	r.Post("/imap/process", h.StartProcess)
	r.Get("/imap/process/stream", h.StreamProgress)
	r.Post("/imap/process/cancel", h.CancelProcess)
	r.Get("/imap/process/status", h.GetStatus)
	r.Post("/imap/folders", h.GetFolders)
}

// ── Request types ─────────────────────────────────────────────────────────────

type imapConnParams struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	UseSSL   bool   `json:"use_ssl"`
}

type imapProcessRequest struct {
	imapConnParams
	Folders        []string `json:"folders"`
	AllFolders     bool     `json:"all_folders"`
	ExcludeFolders []string `json:"exclude_folders"`
	NewOnly        bool     `json:"new_only"`
}

type imapFoldersRequest struct {
	imapConnParams
}

func toConnParams(p imapConnParams) imapimport.ConnParams {
	return imapimport.ConnParams{
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		Password: p.Password,
		UseSSL:   p.UseSSL,
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// StartProcess handles POST /imap/process
func (h *IMAPHandler) StartProcess(w http.ResponseWriter, r *http.Request) {
	if err := h.job.AssertNotRunning(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	var req imapProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Host == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "host, username, and password are required")
		return
	}

	// Resolve folder list with a brief connection.
	folders, err := imapimport.ListFolders(toConnParams(req.imapConnParams))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("IMAP connection failed: %s", err))
		return
	}
	if req.AllFolders {
		// keep all
	} else if len(req.Folders) > 0 {
		folders = req.Folders
	} else {
		folders = []string{"INBOX"}
	}
	if len(req.ExcludeFolders) > 0 {
		filtered := make([]string, 0, len(folders))
		for _, f := range folders {
			excluded := false
			for _, pattern := range req.ExcludeFolders {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if re.MatchString(f) {
					excluded = true
					break
				}
			}
			if !excluded {
				filtered = append(filtered, f)
			}
		}
		folders = filtered
	}
	if len(folders) == 0 {
		writeError(w, http.StatusBadRequest, "no folders found or specified")
		return
	}

	go h.runIMAPImport(req, folders)

	writeJSON(w, map[string]any{
		"message": fmt.Sprintf("IMAP processing started for %d folder(s)", len(folders)),
		"folders": folders,
	})
}

// StreamProgress handles GET /imap/process/stream
func (h *IMAPHandler) StreamProgress(w http.ResponseWriter, r *http.Request) {
	h.job.ServeSSE(w, r)
}

// CancelProcess handles POST /imap/process/cancel
func (h *IMAPHandler) CancelProcess(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.job.Cancel())
}

// GetStatus handles GET /imap/process/status
func (h *IMAPHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.job.Status())
}

// GetFolders handles POST /imap/folders
func (h *IMAPHandler) GetFolders(w http.ResponseWriter, r *http.Request) {
	var req imapFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Host == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "host, username, and password are required")
		return
	}

	folders, err := imapimport.ListFolders(toConnParams(req.imapConnParams))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Failed to connect: %s", err))
		return
	}
	writeJSON(w, map[string]any{"folders": folders})
}

// ── Background import job ─────────────────────────────────────────────────────

func (h *IMAPHandler) runIMAPImport(req imapProcessRequest, folders []string) {
	h.job.Start()
	h.job.UpdateState(map[string]any{
		"status":               "in_progress",
		"status_line":          "Connecting to IMAP server...",
		"current_folder":       nil,
		"current_folder_index": 0,
		"total_folders":        len(folders),
		"emails_processed":     0,
		"folders":              folders,
	})
	h.job.Broadcast("progress", h.job.GetState())
	defer h.job.Finish()

	cancelFn := func() bool { return h.job.IsCancelled() }
	progressFn := func(folder string, folderIdx, totalFolders, emailsProcessed int) {
		h.job.UpdateState(map[string]any{
			"current_folder":       folder,
			"current_folder_index": folderIdx,
			"total_folders":        totalFolders,
			"emails_processed":     emailsProcessed,
			"status_line":          fmt.Sprintf("Processing folder: %s (%d/%d) — %d emails", folder, folderIdx, totalFolders, emailsProcessed),
		})
		h.job.Broadcast("progress", h.job.GetState())
	}

	total, err := imapimport.ImportFolders(
		context.Background(),
		h.pool,
		toConnParams(req.imapConnParams),
		folders,
		req.NewOnly,
		cancelFn,
		progressFn,
	)

	if h.job.IsCancelled() {
		h.job.UpdateState(map[string]any{
			"status":      "cancelled",
			"status_line": "Cancelled by user",
		})
		h.job.Broadcast("cancelled", h.job.GetState())
		return
	}
	if err != nil {
		h.job.UpdateState(map[string]any{
			"status":        "error",
			"error_message": err.Error(),
			"status_line":   err.Error(),
		})
		h.job.Broadcast("error", h.job.GetState())
		return
	}

	h.job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Import completed. %d emails processed.", total),
	})
	runThumbnailsAfterImportIfIdle(h.pool)
	h.job.Broadcast("completed", h.job.GetState())
}

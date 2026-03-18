package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// AttachmentHandler handles all /attachments/* endpoints.
type AttachmentHandler struct {
	svc             *service.AttachmentService
	pythonStaticDir string
}

// NewAttachmentHandler creates an AttachmentHandler.
func NewAttachmentHandler(svc *service.AttachmentService, pythonStaticDir string) *AttachmentHandler {
	return &AttachmentHandler{svc: svc, pythonStaticDir: pythonStaticDir}
}

// RegisterRoutes mounts all attachment routes.
func (h *AttachmentHandler) RegisterRoutes(r chi.Router) {
	// Static page routes (must come before /{id} catch-all)
	r.Get("/attachments-viewer", h.ViewerPage)
	r.Get("/attachments-images-grid", h.ImagesGridPage)

	// Info / navigation routes (must come before /{id})
	r.Get("/attachments/random", h.Random)
	r.Get("/attachments/by-id", h.ByID)
	r.Get("/attachments/by-size", h.BySize)
	r.Get("/attachments/count", h.Count)
	r.Get("/attachments/images", h.ImagesGrid)

	// Per-attachment routes
	r.Get("/attachments/{attachment_id}/info", h.Info)
	r.Get("/attachments/{attachment_id}", h.Content)
	r.Delete("/attachments/{attachment_id}", h.Delete)
}

func parseAttachmentID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "attachment_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_id must be an integer")
		return 0, false
	}
	return id, true
}

func writeAttachmentInfo(a *model.AttachmentInfo) map[string]any {
	m := map[string]any{
		"attachment_id": a.AttachmentID,
		"filename":      a.Filename,
		"content_type":  a.ContentType,
		"size":          a.Size,
		"email_id":      a.EmailID,
		"email_subject": a.EmailSubject,
		"email_from":    a.EmailFrom,
		"email_folder":  a.EmailFolder,
	}
	if a.EmailDate != nil {
		m["email_date"] = a.EmailDate.Format("2006-01-02T15:04:05.999999")
	} else {
		m["email_date"] = nil
	}
	return m
}

// ── Page handlers ─────────────────────────────────────────────────────────────

func (h *AttachmentHandler) ViewerPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(h.pythonStaticDir, "templates", "attachments_viewer.html"))
}

func (h *AttachmentHandler) ImagesGridPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(h.pythonStaticDir, "templates", "images_grid.html"))
}

// ── Data endpoints ────────────────────────────────────────────────────────────

func (h *AttachmentHandler) Random(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.GetRandom(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving attachment: %s", err))
		return
	}
	if a == nil {
		writeJSON(w, nil)
		return
	}
	writeJSON(w, writeAttachmentInfo(a))
}

func (h *AttachmentHandler) ByID(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	a, err := h.svc.GetByIDOrder(r.Context(), offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving attachment: %s", err))
		return
	}
	if a == nil {
		writeJSON(w, nil)
		return
	}
	writeJSON(w, writeAttachmentInfo(a))
}

func (h *AttachmentHandler) BySize(w http.ResponseWriter, r *http.Request) {
	order := r.URL.Query().Get("order")
	orderDesc := strings.ToLower(order) == "desc"
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	a, err := h.svc.GetBySize(r.Context(), orderDesc, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving attachment: %s", err))
		return
	}
	if a == nil {
		writeJSON(w, nil)
		return
	}
	writeJSON(w, writeAttachmentInfo(a))
}

func (h *AttachmentHandler) Count(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error counting attachments: %s", err))
		return
	}
	writeJSON(w, map[string]any{"count": n})
}

func (h *AttachmentHandler) ImagesGrid(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			page = n
		}
	}
	pageSize := 50
	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 100 {
			pageSize = n
		}
	}
	order := q.Get("order")
	if order == "" {
		order = "id"
	}
	direction := q.Get("direction")
	if direction == "" {
		direction = "asc"
	}
	allTypes := q.Get("all_types") == "true" || q.Get("all_types") == "1"

	images, total, err := h.svc.ListImages(r.Context(), page, pageSize, order, direction, allTypes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing images: %s", err))
		return
	}

	out := make([]map[string]any, 0, len(images))
	for _, a := range images {
		out = append(out, writeAttachmentInfo(a))
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	writeJSON(w, map[string]any{
		"images":      out,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
}

func (h *AttachmentHandler) Info(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAttachmentID(w, r)
	if !ok {
		return
	}
	a, err := h.svc.GetInfo(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving attachment info: %s", err))
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("attachment not found: id=%d", id))
		return
	}
	writeJSON(w, writeAttachmentInfo(a))
}

func (h *AttachmentHandler) Content(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAttachmentID(w, r)
	if !ok {
		return
	}
	preview := r.URL.Query().Get("preview") == "true" || r.URL.Query().Get("preview") == "1"

	data, thumbnail, mediaType, filename, err := h.svc.GetData(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving attachment: %s", err))
		return
	}
	if data == nil && thumbnail == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("attachment not found: id=%d", id))
		return
	}

	var content []byte
	var safeName string
	if preview {
		if thumbnail == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("attachment %d has no thumbnail", id))
			return
		}
		mediaType = "image/jpeg"
		base := strings.TrimSuffix(filename, filepath.Ext(filename))
		safeName = strings.ReplaceAll(base+"_thumb.jpg", `"`, `\"`)
		content = thumbnail
	} else {
		if data == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("attachment %d has no content", id))
			return
		}
		safeName = strings.ReplaceAll(filename, `"`, `\"`)
		content = data
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safeName))
	_, _ = w.Write(content)
}

func (h *AttachmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAttachmentID(w, r)
	if !ok {
		return
	}
	deleted, err := h.svc.Delete(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting attachment: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("attachment not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{"message": fmt.Sprintf("Attachment %d deleted successfully", id)})
}

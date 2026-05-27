package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// GuideTopicsHandler handles /api/guide-topics/* endpoints.
type GuideTopicsHandler struct {
	svc                      *service.GuideTopicsService
	reloadFromFileEnabled    bool
	guideTopicsSeedPath      string
}

// NewGuideTopicsHandler creates a GuideTopicsHandler.
func NewGuideTopicsHandler(svc *service.GuideTopicsService, appCfg config.AppConfig) *GuideTopicsHandler {
	return &GuideTopicsHandler{
		svc:                   svc,
		reloadFromFileEnabled: appCfg.GuideTopicsReloadFromFileOnStartup,
		guideTopicsSeedPath:   appCfg.GuideTopicsConfigFile(),
	}
}

// RegisterRoutes mounts guide topic routes.
func (h *GuideTopicsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/guide-topics", h.List)
	r.Get("/api/guide-topics/admin", h.ListAdmin)
	r.Get("/api/guide-topics/export", h.Export)
	r.Post("/api/guide-topics/import/preview", h.ImportPreview)
	r.Post("/api/guide-topics/import", h.ImportApply)
	r.Post("/api/guide-topics/reload-from-file", h.ReloadFromFile)
	r.Post("/api/guide-topics", h.Create)
	r.Delete("/api/guide-topics/all", h.DeleteAll)
	r.Patch("/api/guide-topics/{id}", h.Update)
	r.Delete("/api/guide-topics/{id}", h.Delete)
}

func parseGuideTopicID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func parseGuideTopicInput(w http.ResponseWriter, r *http.Request) (service.GuideTopicInput, bool) {
	var in service.GuideTopicInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return service.GuideTopicInput{}, false
	}
	return in, true
}

// List handles GET /api/guide-topics — returns the topics map for guide.js.
func (h *GuideTopicsHandler) List(w http.ResponseWriter, r *http.Request) {
	doc, err := h.svc.BuildTopicsDocument(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error building guide topics: %s", err))
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, doc)
}

// ListAdmin handles GET /api/guide-topics/admin.
func (h *GuideTopicsHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing guide topics: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, err := service.ParseGuideTopicRow(row)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse guide topic %q: %s", row.Key, err))
			return
		}
		out = append(out, item)
	}
	writeJSON(w, out)
}

// Create handles POST /api/guide-topics.
func (h *GuideTopicsHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := parseGuideTopicInput(w, r)
	if !ok {
		return
	}
	row, err := h.svc.Create(r.Context(), in)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	item, err := service.ParseGuideTopicRow(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, item)
}

// Update handles PATCH /api/guide-topics/{id}.
func (h *GuideTopicsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGuideTopicID(w, r)
	if !ok {
		return
	}
	in, ok := parseGuideTopicInput(w, r)
	if !ok {
		return
	}
	row, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("guide topic not found: id=%d", id))
		return
	}
	item, err := service.ParseGuideTopicRow(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, item)
}

// ReloadFromFile handles POST /api/guide-topics/reload-from-file.
func (h *GuideTopicsHandler) ReloadFromFile(w http.ResponseWriter, r *http.Request) {
	if !h.reloadFromFileEnabled {
		writeError(w, http.StatusForbidden, "guide topics reload from file is not enabled")
		return
	}
	if err := h.svc.ReloadFromFile(r.Context(), h.guideTopicsSeedPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reloading guide topics from file: %s", err))
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// DeleteAll handles DELETE /api/guide-topics/all.
func (h *GuideTopicsHandler) DeleteAll(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.DeleteAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error clearing guide topics: %s", err))
		return
	}
	writeJSON(w, map[string]any{"deleted": n})
}

// Delete handles DELETE /api/guide-topics/{id}.
func (h *GuideTopicsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseGuideTopicID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting guide topic: %s", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Export handles GET /api/guide-topics/export.
func (h *GuideTopicsHandler) Export(w http.ResponseWriter, r *http.Request) {
	doc, err := h.svc.ExportDocument(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error exporting guide topics: %s", err))
		return
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error encoding guide topics export: %s", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="guide_topics.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ImportPreview handles POST /api/guide-topics/import/preview.
func (h *GuideTopicsHandler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	data, ok := readGuideTopicsImportFile(w, r)
	if !ok {
		return
	}
	result, err := h.svc.ImportPreview(r.Context(), data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}

// ImportApply handles POST /api/guide-topics/import.
func (h *GuideTopicsHandler) ImportApply(w http.ResponseWriter, r *http.Request) {
	data, ok := readGuideTopicsImportFile(w, r)
	if !ok {
		return
	}
	resolutions := map[string]string{}
	if raw := strings.TrimSpace(r.FormValue("resolutions")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &resolutions); err != nil {
			writeError(w, http.StatusBadRequest, "invalid resolutions JSON")
			return
		}
	}
	if err := h.svc.ImportApply(r.Context(), data, resolutions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func readGuideTopicsImportFile(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	const maxUpload = 4 << 20
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUpload))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read upload")
		return nil, false
	}
	return data, true
}

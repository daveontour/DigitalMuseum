package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// RegionsHandler handles /api/regions/* admin endpoints.
type RegionsHandler struct {
	svc *service.RegionsService
}

// NewRegionsHandler creates a RegionsHandler.
func NewRegionsHandler(svc *service.RegionsService) *RegionsHandler {
	return &RegionsHandler{svc: svc}
}

// RegisterRoutes mounts region admin routes (GET /api/regions stays on TemplateHandler).
func (h *RegionsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/regions/admin", h.ListAdmin)
	r.Get("/api/regions/export", h.Export)
	r.Post("/api/regions/import/preview", h.ImportPreview)
	r.Post("/api/regions/import", h.ImportApply)
	r.Put("/api/regions/reorder", h.Reorder)
	r.Put("/api/regions/defaults", h.UpdateDefaults)
	r.Post("/api/regions", h.Create)
	r.Patch("/api/regions/{id}", h.Update)
	r.Delete("/api/regions/{id}", h.Delete)
}

func parseRegionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func regionRowResponse(row *georegion.RegionDefinition, id int64, key string, sortOrder int, text string) map[string]any {
	out := map[string]any{
		"id":         id,
		"key":        key,
		"sort_order": sortOrder,
		"text":       text,
	}
	if row != nil {
		out["code"] = row.Code
		out["label"] = row.Label
		out["bbox"] = row.BBox
	}
	return out
}

func parseRegionDefinitionFromBody(w http.ResponseWriter, r *http.Request) (georegion.RegionDefinition, *int, bool) {
	var req struct {
		Code      string    `json:"code"`
		Label     string    `json:"label"`
		BBox      []float64 `json:"bbox"`
		SortOrder *int      `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return georegion.RegionDefinition{}, nil, false
	}
	def := georegion.RegionDefinition{
		Code:  strings.TrimSpace(req.Code),
		Label: strings.TrimSpace(req.Label),
		BBox:  req.BBox,
	}
	return def, req.SortOrder, true
}

// ListAdmin handles GET /api/regions/admin.
func (h *RegionsHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing regions: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if georegion.IsReservedKey(row.Key) {
			out = append(out, map[string]any{
				"id":         row.ID,
				"key":        row.Key,
				"sort_order": row.SortOrder,
				"text":       row.Text,
				"reserved":   true,
			})
			continue
		}
		var def georegion.RegionDefinition
		if err := json.Unmarshal([]byte(row.Text), &def); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse region %q: %s", row.Key, err))
			return
		}
		out = append(out, regionRowResponse(&def, row.ID, row.Key, row.SortOrder, row.Text))
	}
	writeJSON(w, out)
}

// Create handles POST /api/regions.
func (h *RegionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	def, sortOrder, ok := parseRegionDefinitionFromBody(w, r)
	if !ok {
		return
	}
	row, err := h.svc.CreateRegion(r.Context(), def, sortOrder)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, regionRowResponse(&def, row.ID, row.Key, row.SortOrder, row.Text))
}

// Update handles PATCH /api/regions/{id}.
func (h *RegionsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRegionID(w, r)
	if !ok {
		return
	}
	def, sortOrder, ok := parseRegionDefinitionFromBody(w, r)
	if !ok {
		return
	}
	row, err := h.svc.UpdateRegion(r.Context(), id, def, sortOrder)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("region not found: id=%d", id))
		return
	}
	writeJSON(w, regionRowResponse(&def, row.ID, row.Key, row.SortOrder, row.Text))
}

// Delete handles DELETE /api/regions/{id}.
func (h *RegionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRegionID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRegion(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateDefaults handles PUT /api/regions/defaults.
func (h *RegionsHandler) UpdateDefaults(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DefaultRegion string `json:"default_region"`
		DefaultLabel  string `json:"default_label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.svc.UpdateDefaults(r.Context(), req.DefaultRegion, req.DefaultLabel); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{
		"default_region": strings.TrimSpace(req.DefaultRegion),
		"default_label":  strings.TrimSpace(req.DefaultLabel),
	})
}

// Reorder handles PUT /api/regions/reorder.
func (h *RegionsHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []struct {
			ID        int64 `json:"id"`
			SortOrder int   `json:"sort_order"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	items := make([]struct {
		ID        int64
		SortOrder int
	}, len(req.Items))
	for i, item := range req.Items {
		items[i] = struct {
			ID        int64
			SortOrder int
		}{ID: item.ID, SortOrder: item.SortOrder}
	}
	if err := h.svc.Reorder(r.Context(), items); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reordering regions: %s", err))
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// Export handles GET /api/regions/export.
func (h *RegionsHandler) Export(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.ExportConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error exporting regions: %s", err))
		return
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error encoding regions export: %s", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="regions.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ImportPreview handles POST /api/regions/import/preview (multipart file field "file").
func (h *RegionsHandler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	data, ok := readRegionsImportFile(w, r)
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

// ImportApply handles POST /api/regions/import (multipart: file + optional resolutions JSON field).
func (h *RegionsHandler) ImportApply(w http.ResponseWriter, r *http.Request) {
	data, ok := readRegionsImportFile(w, r)
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

func readRegionsImportFile(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	const maxUpload = 2 << 20
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxUpload))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read upload")
		return nil, false
	}
	return data, true
}

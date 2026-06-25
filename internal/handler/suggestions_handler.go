package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// SuggestionsHandler handles /api/suggestions/* admin endpoints.
type SuggestionsHandler struct {
	svc *service.SuggestionsService
}

// NewSuggestionsHandler creates a SuggestionsHandler.
func NewSuggestionsHandler(svc *service.SuggestionsService) *SuggestionsHandler {
	return &SuggestionsHandler{svc: svc}
}

// RegisterRoutes mounts suggestion admin routes (GET /api/suggestions stays on TemplateHandler).
func (h *SuggestionsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/suggestions/admin", h.ListAdmin)
	r.Get("/api/suggestions/export", h.Export)
	r.Post("/api/suggestions/import/preview", h.ImportPreview)
	r.Post("/api/suggestions/import", h.ImportApply)
	r.Post("/api/suggestions", h.Create)
	r.Patch("/api/suggestions/{id}", h.Update)
	r.Delete("/api/suggestions/{id}", h.Delete)
}

func parseSuggestionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func parseSuggestionInput(w http.ResponseWriter, r *http.Request) (service.SuggestionInput, bool) {
	var in service.SuggestionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return service.SuggestionInput{}, false
	}
	return in, true
}

func suggestionAdminResponse(row *service.SuggestionInput, id int64, key, text string) map[string]any {
	out := map[string]any{
		"id":   id,
		"key":  key,
		"text": text,
	}
	if row != nil {
		out["category"] = row.Category
		out["title"] = row.Title
		out["prompt"] = row.Prompt
		if row.Description != "" {
			out["description"] = row.Description
		}
		if row.ActionLabel != "" {
			out["action_label"] = row.ActionLabel
		}
		if len(row.Requires) > 0 {
			out["requires"] = row.Requires
		}
		if row.Sensitivity != "" {
			out["sensitivity"] = row.Sensitivity
		}
		if row.Function != "" {
			out["function"] = row.Function
		}
	}
	return out
}

func rowToAdminResponse(row *model.SuggestionRow) (map[string]any, error) {
	category, item, err := service.ParseSuggestionRow(row)
	if err != nil {
		return nil, err
	}
	in := service.SuggestionInput{Category: category}
	if t, ok := item["title"].(string); ok {
		in.Title = t
	}
	if p, ok := item["prompt"].(string); ok {
		in.Prompt = p
	}
	if d, ok := item["description"].(string); ok {
		in.Description = d
	}
	if a, ok := item["action_label"].(string); ok {
		in.ActionLabel = a
	}
	if req, ok := item["requires"].([]any); ok {
		for _, v := range req {
			if s, ok := v.(string); ok {
				in.Requires = append(in.Requires, s)
			}
		}
	}
	if sens, ok := item["sensitivity"].(string); ok {
		in.Sensitivity = sens
	}
	if fn, ok := item["function"].(string); ok {
		in.Function = fn
	}
	return suggestionAdminResponse(&in, row.ID, row.Key, row.Text), nil
}

// ListAdmin handles GET /api/suggestions/admin.
func (h *SuggestionsHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing suggestions: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		category, item, err := service.ParseSuggestionRow(row)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse suggestion %q: %s", row.Key, err))
			return
		}
		in := service.SuggestionInput{Category: category}
		if t, ok := item["title"].(string); ok {
			in.Title = t
		}
		if p, ok := item["prompt"].(string); ok {
			in.Prompt = p
		}
		if d, ok := item["description"].(string); ok {
			in.Description = d
		}
		if a, ok := item["action_label"].(string); ok {
			in.ActionLabel = a
		}
		if req, ok := item["requires"].([]any); ok {
			for _, v := range req {
				if s, ok := v.(string); ok {
					in.Requires = append(in.Requires, s)
				}
			}
		}
		if sens, ok := item["sensitivity"].(string); ok {
			in.Sensitivity = sens
		}
		if fn, ok := item["function"].(string); ok {
			in.Function = fn
		}
		out = append(out, suggestionAdminResponse(&in, row.ID, row.Key, row.Text))
	}
	writeJSON(w, out)
}

// Create handles POST /api/suggestions.
func (h *SuggestionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := parseSuggestionInput(w, r)
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
	resp, err := rowToAdminResponse(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// Update handles PATCH /api/suggestions/{id}.
func (h *SuggestionsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSuggestionID(w, r)
	if !ok {
		return
	}
	in, ok := parseSuggestionInput(w, r)
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
		writeError(w, http.StatusNotFound, fmt.Sprintf("suggestion not found: id=%d", id))
		return
	}
	resp, err := rowToAdminResponse(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// Delete handles DELETE /api/suggestions/{id}.
func (h *SuggestionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSuggestionID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting suggestion: %s", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Export handles GET /api/suggestions/export.
func (h *SuggestionsHandler) Export(w http.ResponseWriter, r *http.Request) {
	doc, err := h.svc.ExportDocument(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error exporting suggestions: %s", err))
		return
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error encoding suggestions export: %s", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="suggestions.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ImportPreview handles POST /api/suggestions/import/preview.
func (h *SuggestionsHandler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	data, ok := readSuggestionsImportFile(w, r)
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

// ImportApply handles POST /api/suggestions/import.
func (h *SuggestionsHandler) ImportApply(w http.ResponseWriter, r *http.Request) {
	data, ok := readSuggestionsImportFile(w, r)
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

func readSuggestionsImportFile(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
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
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxUpload))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read upload")
		return nil, false
	}
	return data, true
}

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// AIModelsHandler handles /api/ai-models/* endpoints.
type AIModelsHandler struct {
	svc        *service.AIModelsService
	catalogSvc *service.OpenRouterCatalogService
}

// NewAIModelsHandler creates an AIModelsHandler.
func NewAIModelsHandler(svc *service.AIModelsService, catalogSvc *service.OpenRouterCatalogService) *AIModelsHandler {
	return &AIModelsHandler{svc: svc, catalogSvc: catalogSvc}
}

// RegisterRoutes mounts AI model routes.
func (h *AIModelsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/ai-models", h.ListAdmin)
	r.Get("/api/ai-models/available", h.ListAvailable)
	r.Get("/api/ai-models/catalog", h.Catalog)
	r.Post("/api/ai-models", h.Create)
	r.Patch("/api/ai-models/{id}", h.Update)
	r.Delete("/api/ai-models/{id}", h.Delete)
}

func parseAIModelID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func parseAIModelInput(w http.ResponseWriter, r *http.Request) (service.AIModelInput, bool) {
	var in service.AIModelInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return service.AIModelInput{}, false
	}
	return in, true
}

func aiModelJSON(m *service.AIModel) map[string]any {
	return map[string]any{
		"id":           m.ID,
		"key":          m.Key,
		"display_name": m.DisplayName,
		"model_slug":   m.ModelSlug,
		"enabled":      m.Enabled,
		"sort_order":   m.SortOrder,
	}
}

// ListAdmin handles GET /api/ai-models — full admin table listing.
func (h *AIModelsHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	models, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing ai models: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		mm := m
		out = append(out, aiModelJSON(&mm))
	}
	writeJSON(w, map[string]any{"models": out})
}

// ListAvailable handles GET /api/ai-models/available — enabled models in table sort_order
// (including localai when enabled), for runtime provider pickers.
func (h *AIModelsHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	models, err := h.svc.ListEnabledInTableOrder(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing available ai models: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		mm := m
		out = append(out, aiModelJSON(&mm))
	}
	defaultKey, _ := h.svc.DefaultKey(r.Context())
	writeJSON(w, map[string]any{"models": out, "default_model_key": defaultKey})
}

func strOrEmpty(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

// Catalog handles GET /api/ai-models/catalog — the full OpenRouter model
// catalog (distinct from the admin-curated list in ListAdmin/ListAvailable),
// for the Configuration → Model Catalog browser.
func (h *AIModelsHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	models, err := h.catalogSvc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("error fetching OpenRouter model catalog: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		out = append(out, map[string]any{
			"id":                m.ID,
			"canonical_slug":    m.CanonicalSlug,
			"name":              m.Name,
			"description":       m.Description,
			"created":           m.Created,
			"context_length":    m.ContextLength,
			"modality":          m.Architecture.Modality,
			"input_modalities":  strOrEmpty(m.Architecture.InputModalities),
			"output_modalities": strOrEmpty(m.Architecture.OutputModalities),
			"tokenizer":         m.Architecture.Tokenizer,
			"instruct_type":     m.Architecture.InstructType,
			"pricing": map[string]any{
				"prompt":            m.Pricing.Prompt,
				"completion":        m.Pricing.Completion,
				"request":           m.Pricing.Request,
				"image":             m.Pricing.Image,
				"web_search":        m.Pricing.WebSearch,
				"input_cache_read":  m.Pricing.InputCacheRead,
				"input_cache_write": m.Pricing.InputCacheWrite,
			},
			"max_completion_tokens": m.TopProvider.MaxCompletionTokens,
			"is_moderated":          m.TopProvider.IsModerated,
			"supported_parameters":  strOrEmpty(m.SupportedParameters),
		})
	}
	writeJSON(w, map[string]any{"models": out})
}

// Create handles POST /api/ai-models.
func (h *AIModelsHandler) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := parseAIModelInput(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Create(r.Context(), in)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, aiModelJSON(m))
}

// Update handles PATCH /api/ai-models/{id}.
func (h *AIModelsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAIModelID(w, r)
	if !ok {
		return
	}
	in, ok := parseAIModelInput(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("ai model not found: id=%d", id))
		return
	}
	writeJSON(w, aiModelJSON(m))
}

// Delete handles DELETE /api/ai-models/{id}.
func (h *AIModelsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAIModelID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting ai model: %s", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

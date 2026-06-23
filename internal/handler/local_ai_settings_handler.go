package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// LocalAISettingsHandler serves machine-wide Local AI settings and Ollama model list.
type LocalAISettingsHandler struct {
	chatSvc *service.ChatService
	baseURL string
}

// NewLocalAISettingsHandler creates a handler. chatSvc may be nil (minimal router).
func NewLocalAISettingsHandler(chatSvc *service.ChatService, baseURL string) *LocalAISettingsHandler {
	return &LocalAISettingsHandler{chatSvc: chatSvc, baseURL: strings.TrimSpace(baseURL)}
}

// RegisterRoutes mounts Local AI settings endpoints (authenticated).
func (h *LocalAISettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/local-ai/settings", h.GetSettings)
	r.Get("/api/local-ai/models", h.ListModels)
	r.Post("/api/local-ai/settings", h.ApplySettings)
}

// GET /api/local-ai/settings
func (h *LocalAISettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, config.LocalAIRuntimeStore().Snapshot())
}

// GET /api/local-ai/models
func (h *LocalAISettingsHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	baseURL := h.localAIBaseURL()
	if baseURL == "" {
		writeError(w, http.StatusServiceUnavailable, "LOCALAI_BASE_URL is not configured")
		return
	}
	models, err := appai.ListOllamaModels(r.Context(), baseURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if models == nil {
		models = []string{}
	}
	writeJSON(w, map[string]any{"models": models})
}

// POST /api/local-ai/settings — apply chat model and CUDA CPU-only preference.
func (h *LocalAISettingsHandler) ApplySettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChatModel   string `json:"chat_model"`
		CudaCPUOnly bool   `json:"cuda_cpu_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := config.LocalAIRuntimeStore().Apply(strings.TrimSpace(req.ChatModel), req.CudaCPUOnly); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.chatSvc != nil {
		h.chatSvc.InvalidateLocalAIProbeCache()
	}
	writeJSON(w, config.LocalAIRuntimeStore().Snapshot())
}

func (h *LocalAISettingsHandler) localAIBaseURL() string {
	if h.chatSvc != nil {
		if u := h.chatSvc.LocalAIBaseURL(); u != "" {
			return u
		}
	}
	return h.baseURL
}

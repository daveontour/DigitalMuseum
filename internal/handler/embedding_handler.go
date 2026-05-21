package handler

import (
	"encoding/json"
	"net/http"

	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// EmbeddingHandler handles POST /api/embed.
type EmbeddingHandler struct {
	svc *service.EmbeddingService
}

// NewEmbeddingHandler creates an EmbeddingHandler.
func NewEmbeddingHandler(svc *service.EmbeddingService) *EmbeddingHandler {
	return &EmbeddingHandler{svc: svc}
}

// RegisterRoutes mounts the embedding routes.
func (h *EmbeddingHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/embed/availability", h.Availability)
	r.Post("/api/embed", h.Embed)
	r.Post("/api/embed/batch", h.EmbedBatch)
}

// GET /api/embed/availability
func (h *EmbeddingHandler) Availability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"available": h.svc.IsAvailable()})
}

// POST /api/embed
// Body: { "text": "..." }
// Response: { "embedding": [float, ...], "dimensions": N }
func (h *EmbeddingHandler) Embed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if !h.svc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	vec, err := h.svc.EmbedText(r.Context(), req.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "embedding failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"embedding":  vec,
		"dimensions": len(vec),
	})
}

// POST /api/embed/batch
// Body: { "texts": ["...", "..."] }
// Response: { "embeddings": [[float, ...], ...], "dimensions": N, "count": N }
func (h *EmbeddingHandler) EmbedBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Texts []string `json:"texts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Texts) == 0 {
		writeError(w, http.StatusBadRequest, "texts must be a non-empty array")
		return
	}
	if !h.svc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	vecs, err := h.svc.EmbedBatch(r.Context(), req.Texts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch embedding failed: "+err.Error())
		return
	}
	dims := 0
	if len(vecs) > 0 {
		dims = len(vecs[0])
	}
	writeJSON(w, map[string]any{
		"embeddings": vecs,
		"dimensions": dims,
		"count":      len(vecs),
	})
}

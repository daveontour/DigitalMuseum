package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ConfigHandler handles all /api/configuration/* endpoints.
type ConfigHandler struct {
	svc *service.ConfigService
}

// NewConfigHandler creates a ConfigHandler.
func NewConfigHandler(svc *service.ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

// RegisterRoutes mounts all configuration routes.
func (h *ConfigHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/configuration", h.List)
	r.Post("/api/configuration", h.Upsert)
	r.Post("/api/configuration/seed", h.Seed)
	r.Delete("/api/configuration/{key}", h.Delete)
}

// ── List ──────────────────────────────────────────────────────────────────────

func (h *ConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing configuration: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, c := range items {
		out = append(out, map[string]any{
			"id":           c.ID,
			"key":          c.Key,
			"value":        c.Value,
			"is_mandatory": c.IsMandatory,
			"description":  c.Description,
			"created_at":   c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			"updated_at":   c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, out)
}

// ── Upsert ────────────────────────────────────────────────────────────────────

func (h *ConfigHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key         string  `json:"key"`
		Value       *string `json:"value"`
		IsMandatory *bool   `json:"is_mandatory"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key must not be empty")
		return
	}
	c, err := h.svc.Upsert(r.Context(), req.Key, req.Value, req.IsMandatory, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving configuration: %s", err))
		return
	}
	writeJSON(w, map[string]any{
		"id":           c.ID,
		"key":          c.Key,
		"value":        c.Value,
		"is_mandatory": c.IsMandatory,
		"description":  c.Description,
		"created_at":   c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":   c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *ConfigHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	deleted, err := h.svc.Delete(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting configuration: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("key '%s' not found in database", key))
		return
	}
	writeJSON(w, map[string]any{"deleted": true, "key": key})
}

// ── Seed ──────────────────────────────────────────────────────────────────────

func (h *ConfigHandler) Seed(w http.ResponseWriter, r *http.Request) {
	count, err := h.svc.SeedFromEnv(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error seeding configuration: %s", err))
		return
	}
	writeJSON(w, map[string]any{
		"seeded":  count,
		"message": fmt.Sprintf("Seeded %d new key(s) from .env", count),
	})
}

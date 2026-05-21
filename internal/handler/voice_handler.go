package handler

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// VoiceHandler handles all /api/voices/* endpoints.
type VoiceHandler struct {
	svc          *service.VoiceService
	sessionStore *keystore.SessionMasterStore
}

// NewVoiceHandler creates a VoiceHandler.
func NewVoiceHandler(svc *service.VoiceService, sessionStore *keystore.SessionMasterStore) *VoiceHandler {
	return &VoiceHandler{svc: svc, sessionStore: sessionStore}
}

// RegisterRoutes mounts all voice routes.
func (h *VoiceHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/voices", h.ListAll)
	r.Get("/api/voices/custom", h.ListCustom)
	r.Post("/api/voices", h.Create)
	r.Put("/api/voices/{voice_id}", h.Update)
	r.Delete("/api/voices/{voice_id}", h.Delete)
}

func parseVoiceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "voice_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "voice_id must be an integer")
		return 0, false
	}
	return id, true
}

// ── List all (built-in + custom) ──────────────────────────────────────────────

func (h *VoiceHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing voices: %s", err))
		return
	}
	if result == nil {
		result = []map[string]any{}
	}
	writeJSON(w, result)
}

// ── List custom only ──────────────────────────────────────────────────────────

func (h *VoiceHandler) ListCustom(w http.ResponseWriter, r *http.Request) {
	voices, err := h.svc.ListCustom(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing custom voices: %s", err))
		return
	}
	type voiceJSON struct {
		ID           int64   `json:"id"`
		Key          string  `json:"key"`
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		Instructions string  `json:"instructions"`
		Creativity   float64 `json:"creativity"`
		IsCustom     bool    `json:"is_custom"`
		CreatedAt    string  `json:"created_at"`
		UpdatedAt    string  `json:"updated_at"`
	}
	out := make([]voiceJSON, 0, len(voices))
	for _, v := range voices {
		out = append(out, voiceJSON{
			ID: v.ID, Key: v.Key, Name: v.Name, Description: v.Description,
			Instructions: v.Instructions, Creativity: v.Creativity, IsCustom: true,
			CreatedAt: v.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			UpdatedAt: v.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, out)
}

// ── Create ────────────────────────────────────────────────────────────────────

func (h *VoiceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		Instructions string  `json:"instructions"`
		Creativity   float64 `json:"creativity"`
	}
	req.Creativity = 0.5
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	req.Instructions = strings.TrimSpace(req.Instructions)
	if req.Instructions == "" {
		writeError(w, http.StatusBadRequest, "instructions are required")
		return
	}
	creativity := math.Max(0.0, math.Min(2.0, req.Creativity))

	v, err := h.svc.Create(r.Context(), req.Name, req.Description, req.Instructions, creativity)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating voice: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"id": v.ID, "key": v.Key, "name": v.Name, "description": v.Description,
		"instructions": v.Instructions, "creativity": v.Creativity, "is_custom": true,
		"created_at": v.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at": v.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

// ── Update ────────────────────────────────────────────────────────────────────

func (h *VoiceHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseVoiceID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name         *string  `json:"name"`
		Description  *string  `json:"description"`
		Instructions *string  `json:"instructions"`
		Creativity   *float64 `json:"creativity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "name must not be empty")
			return
		}
		req.Name = &trimmed
	}
	if req.Instructions != nil {
		trimmed := strings.TrimSpace(*req.Instructions)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "instructions must not be empty")
			return
		}
		req.Instructions = &trimmed
	}
	if req.Creativity != nil {
		c := math.Max(0.0, math.Min(2.0, *req.Creativity))
		req.Creativity = &c
	}

	v, err := h.svc.Update(r.Context(), id, req.Name, req.Description, req.Instructions, req.Creativity)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating voice: %s", err))
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("custom voice not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id": v.ID, "key": v.Key, "name": v.Name, "description": v.Description,
		"instructions": v.Instructions, "creativity": v.Creativity, "is_custom": true,
		"created_at": v.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at": v.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *VoiceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseVoiceID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving voice: %s", err))
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("custom voice not found: id=%d", id))
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting voice: %s", err))
		return
	}
	writeJSON(w, map[string]any{"message": "Custom voice deleted", "id": id})
}

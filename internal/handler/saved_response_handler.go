package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// SavedResponseHandler handles all /api/saved-responses/* endpoints.
type SavedResponseHandler struct {
	svc          *service.SavedResponseService
	sessionStore *keystore.SessionMasterStore
}

// NewSavedResponseHandler creates a SavedResponseHandler.
func NewSavedResponseHandler(svc *service.SavedResponseService, sessionStore *keystore.SessionMasterStore) *SavedResponseHandler {
	return &SavedResponseHandler{svc: svc, sessionStore: sessionStore}
}

// RegisterRoutes mounts all saved response routes.
func (h *SavedResponseHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/saved-responses", h.List)
	r.Post("/api/saved-responses", h.Create)
	r.Get("/api/saved-responses/{id}", h.GetByID)
	r.Put("/api/saved-responses/{id}", h.Update)
	r.Delete("/api/saved-responses/{id}", h.Delete)
}

func parseSavedResponseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func savedResponseMap(id int64, title, content string, voice, llmProvider *string, createdAt string) map[string]any {
	return map[string]any{
		"id":           id,
		"title":        title,
		"content":      content,
		"voice":        voice,
		"llm_provider": llmProvider,
		"created_at":   createdAt,
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func (h *SavedResponseHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing saved responses: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, s := range rows {
		out = append(out, savedResponseMap(s.ID, s.Title, s.Content, s.Voice, s.LLMProvider,
			s.CreatedAt.Format("2006-01-02T15:04:05.999999")))
	}
	writeJSON(w, out)
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func (h *SavedResponseHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSavedResponseID(w, r)
	if !ok {
		return
	}
	s, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving saved response: %s", err))
		return
	}
	if s == nil {
		writeError(w, http.StatusNotFound, "saved response not found")
		return
	}
	writeJSON(w, savedResponseMap(s.ID, s.Title, s.Content, s.Voice, s.LLMProvider,
		s.CreatedAt.Format("2006-01-02T15:04:05.999999")))
}

// ── Create ────────────────────────────────────────────────────────────────────

func (h *SavedResponseHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req struct {
		Title       string  `json:"title"`
		Content     string  `json:"content"`
		Voice       *string `json:"voice"`
		LLMProvider *string `json:"llm_provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title must not be empty")
		return
	}
	s, err := h.svc.Create(r.Context(), strings.TrimSpace(req.Title), req.Content, req.Voice, req.LLMProvider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating saved response: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, savedResponseMap(s.ID, s.Title, s.Content, s.Voice, s.LLMProvider,
		s.CreatedAt.Format("2006-01-02T15:04:05.999999")))
}

// ── Update ────────────────────────────────────────────────────────────────────

func (h *SavedResponseHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSavedResponseID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Content     *string `json:"content"`
		Voice       *string `json:"voice"`
		LLMProvider *string `json:"llm_provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	s, err := h.svc.Update(r.Context(), id, req.Title, req.Content, req.Voice, req.LLMProvider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating saved response: %s", err))
		return
	}
	if s == nil {
		writeError(w, http.StatusNotFound, "saved response not found")
		return
	}
	writeJSON(w, savedResponseMap(s.ID, s.Title, s.Content, s.Voice, s.LLMProvider,
		s.CreatedAt.Format("2006-01-02T15:04:05.999999")))
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *SavedResponseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseSavedResponseID(w, r)
	if !ok {
		return
	}
	deleted, err := h.svc.Delete(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting saved response: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "saved response not found")
		return
	}
	writeJSON(w, map[string]any{"deleted": true, "id": id})
}

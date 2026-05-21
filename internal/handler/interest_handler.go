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

// InterestHandler handles all /api/interests/* endpoints.
type InterestHandler struct {
	svc          *service.InterestService
	sessionStore *keystore.SessionMasterStore
}

// NewInterestHandler creates an InterestHandler.
func NewInterestHandler(svc *service.InterestService, sessionStore *keystore.SessionMasterStore) *InterestHandler {
	return &InterestHandler{svc: svc, sessionStore: sessionStore}
}

// RegisterRoutes mounts all interest routes.
func (h *InterestHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/interests", h.List)
	r.Post("/api/interests", h.Create)
	r.Get("/api/interests/{interest_id}", h.GetByID)
	r.Put("/api/interests/{interest_id}", h.Update)
	r.Delete("/api/interests/{interest_id}", h.Delete)
}

func parseInterestID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "interest_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "interest_id must be an integer")
		return 0, false
	}
	return id, true
}

func interestResponse(id int64, name, createdAt, updatedAt string) map[string]any {
	return map[string]any{
		"id":         id,
		"name":       name,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func (h *InterestHandler) List(w http.ResponseWriter, r *http.Request) {
	interests, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing interests: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(interests))
	for _, i := range interests {
		out = append(out, interestResponse(i.ID, i.Name,
			i.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			i.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		))
	}
	writeJSON(w, out)
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func (h *InterestHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInterestID(w, r)
	if !ok {
		return
	}
	i, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving interest: %s", err))
		return
	}
	if i == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("interest not found: id=%d", id))
		return
	}
	writeJSON(w, interestResponse(i.ID, i.Name,
		i.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		i.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	))
}

// ── Create ────────────────────────────────────────────────────────────────────

func (h *InterestHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	i, err := h.svc.Create(r.Context(), req.Name)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating interest: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, interestResponse(i.ID, i.Name,
		i.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		i.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	))
}

// ── Update ────────────────────────────────────────────────────────────────────

func (h *InterestHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInterestID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	i, err := h.svc.Update(r.Context(), id, req.Name)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating interest: %s", err))
		return
	}
	if i == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("interest not found: id=%d", id))
		return
	}
	writeJSON(w, interestResponse(i.ID, i.Name,
		i.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		i.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	))
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *InterestHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseInterestID(w, r)
	if !ok {
		return
	}
	i, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving interest: %s", err))
		return
	}
	if i == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("interest not found: id=%d", id))
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting interest: %s", err))
		return
	}
	writeJSON(w, map[string]any{"message": "Interest deleted", "id": id})
}

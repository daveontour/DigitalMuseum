package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// PathEquivalenceHandler exposes CRUD for filesystem-import path-equivalence
// rules (see internal/import/filesystem's ExpandEquivalentPaths for how
// these rules are actually applied during import). Local file-path config,
// not sensitive personal data, so this uses the same authentication tier as
// the import endpoints themselves — no special visitor-tier gate.
type PathEquivalenceHandler struct {
	svc *service.PathEquivalenceService
}

// NewPathEquivalenceHandler creates a PathEquivalenceHandler.
func NewPathEquivalenceHandler(svc *service.PathEquivalenceService) *PathEquivalenceHandler {
	return &PathEquivalenceHandler{svc: svc}
}

// RegisterRoutes mounts the path-equivalence CRUD routes.
func (h *PathEquivalenceHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/path-equivalences", h.List)
	r.Post("/api/path-equivalences", h.Create)
	r.Put("/api/path-equivalences/{id}", h.Update)
	r.Delete("/api/path-equivalences/{id}", h.Delete)
}

func pathEquivalenceJSON(p *model.PathEquivalence) map[string]any {
	return map[string]any{"id": p.ID, "path_a": p.PathA, "path_b": p.PathB}
}

func (h *PathEquivalenceHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing path equivalences: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		out = append(out, pathEquivalenceJSON(p))
	}
	writeJSON(w, map[string]any{"rules": out})
}

type pathEquivalenceRequestBody struct {
	PathA string `json:"path_a"`
	PathB string `json:"path_b"`
}

func decodePathEquivalenceBody(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	var req pathEquivalenceRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return "", "", false
	}
	pathA := strings.TrimSpace(req.PathA)
	pathB := strings.TrimSpace(req.PathB)
	if pathA == "" || pathB == "" {
		writeError(w, http.StatusBadRequest, "path_a and path_b are required")
		return "", "", false
	}
	if strings.EqualFold(pathA, pathB) {
		writeError(w, http.StatusBadRequest, "path_a and path_b must be different directories")
		return "", "", false
	}
	return pathA, pathB, true
}

func (h *PathEquivalenceHandler) Create(w http.ResponseWriter, r *http.Request) {
	pathA, pathB, ok := decodePathEquivalenceBody(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Create(r.Context(), pathA, pathB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating path equivalence: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, pathEquivalenceJSON(p))
}

func (h *PathEquivalenceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	pathA, pathB, ok := decodePathEquivalenceBody(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Update(r.Context(), id, pathA, pathB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating path equivalence: %s", err))
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("path equivalence %d not found", id))
		return
	}
	writeJSON(w, pathEquivalenceJSON(p))
}

func (h *PathEquivalenceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return
	}
	deleted, err := h.svc.Delete(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting path equivalence: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("path equivalence %d not found", id))
		return
	}
	writeJSON(w, map[string]any{"deleted": true, "id": id})
}

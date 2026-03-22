package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ArtefactHandler handles all /artefacts/* endpoints.
type ArtefactHandler struct {
	svc *service.ArtefactService
}

// NewArtefactHandler creates an ArtefactHandler.
func NewArtefactHandler(svc *service.ArtefactService) *ArtefactHandler {
	return &ArtefactHandler{svc: svc}
}

// RegisterRoutes mounts all artefact routes onto r.
func (h *ArtefactHandler) RegisterRoutes(r chi.Router) {
	// Specific paths before parameterised routes
	r.Get("/artefacts/export", h.Export)
	r.Post("/artefacts/import", h.Import)
	r.Get("/artefacts", h.List)
	r.Post("/artefacts", h.Create)

	r.Get("/artefacts/{artefact_id}", h.GetByID)
	r.Put("/artefacts/{artefact_id}", h.Update)
	r.Delete("/artefacts/{artefact_id}", h.Delete)
	r.Get("/artefacts/{artefact_id}/thumbnail", h.GetThumbnail)

	r.Post("/artefacts/{artefact_id}/media/upload", h.UploadMedia)
	r.Post("/artefacts/{artefact_id}/media/{media_item_id}", h.LinkMedia)
	r.Delete("/artefacts/{artefact_id}/media/{media_item_id}", h.UnlinkMedia)
}

// ── List ──────────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	tags := r.URL.Query().Get("tags")

	summaries, err := h.svc.List(r.Context(), search, tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing artefacts: %s", err))
		return
	}

	// Build JSON-serialisable slice preserving nil for optional fields
	type summaryJSON struct {
		ID                  int64   `json:"id"`
		Name                string  `json:"name"`
		Description         *string `json:"description"`
		Tags                *string `json:"tags"`
		CreatedAt           string  `json:"created_at"`
		UpdatedAt           string  `json:"updated_at"`
		PrimaryThumbnailURL *string `json:"primary_thumbnail_url"`
	}
	out := make([]summaryJSON, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, summaryJSON{
			ID:                  s.ID,
			Name:                s.Name,
			Description:         s.Description,
			Tags:                s.Tags,
			CreatedAt:           s.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			UpdatedAt:           s.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
			PrimaryThumbnailURL: s.PrimaryThumbnailURL,
		})
	}
	writeJSON(w, out)
}

// ── Export ────────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) Export(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ExportAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("export failed: %s", err))
		return
	}

	type mediaRefJSON struct {
		SortOrder       int     `json:"sort_order"`
		MediaType       *string `json:"media_type"`
		Title           *string `json:"title"`
		Source          *string `json:"source"`
		SourceReference *string `json:"source_reference"`
	}
	type artefactExportJSON struct {
		Name        string         `json:"name"`
		Description *string        `json:"description"`
		Tags        *string        `json:"tags"`
		Story       *string        `json:"story"`
		CreatedAt   *string        `json:"created_at"`
		MediaRefs   []mediaRefJSON `json:"media_refs"`
	}

	artefacts := make([]artefactExportJSON, 0, len(rows))
	for _, row := range rows {
		refs := make([]mediaRefJSON, 0, len(row.MediaRefs))
		for _, ref := range row.MediaRefs {
			refs = append(refs, mediaRefJSON{
				SortOrder:       ref.SortOrder,
				MediaType:       ref.MediaType,
				Title:           ref.Title,
				Source:          ref.Source,
				SourceReference: ref.SourceReference,
			})
		}
		ts := row.Artefact.CreatedAt.Format("2006-01-02T15:04:05.999999")
		artefacts = append(artefacts, artefactExportJSON{
			Name:        row.Artefact.Name,
			Description: row.Artefact.Description,
			Tags:        row.Artefact.Tags,
			Story:       row.Artefact.Story,
			CreatedAt:   &ts,
			MediaRefs:   refs,
		})
	}

	payload, err := json.MarshalIndent(map[string]any{
		"version":   1,
		"artefacts": artefacts,
	}, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "serialisation error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="artefacts_export.json"`)
	_, _ = w.Write(payload)
}

// ── Import ────────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) Import(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	// Support multipart/form-data (file upload) or raw JSON body
	if ct := r.Header.Get("Content-Type"); strings.HasPrefix(ct, "multipart/form-data") {
		if err2 := r.ParseMultipartForm(32 << 20); err2 == nil {
			f, _, ferr := r.FormFile("file")
			if ferr == nil {
				raw, err = io.ReadAll(f)
				f.Close()
				if err != nil {
					writeError(w, http.StatusBadRequest, "could not read uploaded file")
					return
				}
			}
		}
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON file")
		return
	}
	artefactsRaw, ok := data["artefacts"]
	if !ok {
		writeError(w, http.StatusBadRequest, "JSON must contain an 'artefacts' key")
		return
	}
	artefactList, ok := artefactsRaw.([]any)
	if !ok {
		writeError(w, http.StatusBadRequest, "'artefacts' must be a list")
		return
	}

	items := make([]map[string]any, 0, len(artefactList))
	for _, v := range artefactList {
		if m, ok := v.(map[string]any); ok {
			items = append(items, m)
		}
	}

	created, linked, skipped, err := h.svc.ImportArtefacts(r.Context(), items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("import failed: %s", err))
		return
	}

	writeJSON(w, map[string]any{
		"message": fmt.Sprintf(
			"Import complete: %d artefact(s) created, %d photo link(s) restored, %d photo reference(s) skipped (images not found in this system).",
			created, linked, skipped,
		),
		"created":       created,
		"linked_media":  linked,
		"skipped_media": skipped,
	})
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseArtefactID(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving artefact: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("artefact %d not found", id))
		return
	}
	writeJSON(w, resp)
}

// ── Create ────────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Tags        *string `json:"tags"`
		Story       *string `json:"story"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "artefact name is required")
		return
	}

	resp, err := h.svc.Create(r.Context(), req.Name, req.Description, req.Tags, req.Story)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating artefact: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, resp)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseArtefactID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Tags        *string `json:"tags"`
		Story       *string `json:"story"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "artefact name cannot be empty")
			return
		}
		req.Name = &trimmed
	}

	resp, err := h.svc.Update(r.Context(), id, req.Name, req.Description, req.Tags, req.Story)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating artefact: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("artefact %d not found", id))
		return
	}
	writeJSON(w, resp)
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseArtefactID(w, r)
	if !ok {
		return
	}
	// Verify exists first
	a, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving artefact: %s", err))
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("artefact %d not found", id))
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting artefact: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": fmt.Sprintf("Artefact %d deleted successfully", id)})
}

// ── GetThumbnail ──────────────────────────────────────────────────────────────

func (h *ArtefactHandler) GetThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseArtefactID(w, r)
	if !ok {
		return
	}
	data, err := h.svc.GetThumbnail(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving thumbnail: %s", err))
		return
	}
	if data == nil {
		writeError(w, http.StatusNotFound, "no media found for this artefact")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(data)
}

// ── UploadMedia ───────────────────────────────────────────────────────────────

func (h *ArtefactHandler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := parseArtefactID(w, r)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "could not parse multipart form")
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer f.Close()

	imageBytes, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read uploaded file")
		return
	}
	if len(imageBytes) == 0 {
		writeError(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = fh.Filename
	}
	if title == "" {
		title = fmt.Sprintf("artefact_%d_image", id)
	}

	mediaType := fh.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "image/jpeg"
	}

	resp, err := h.svc.UploadMedia(r.Context(), id, imageBytes, title, mediaType)
	if err != nil {
		if errors.Is(err, service.ErrArtefactUnsupportedMedia) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error uploading media: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("artefact %d not found", id))
		return
	}
	writeJSON(w, resp)
}

// ── LinkMedia ─────────────────────────────────────────────────────────────────

func (h *ArtefactHandler) LinkMedia(w http.ResponseWriter, r *http.Request) {
	artefactID, ok := parseArtefactID(w, r)
	if !ok {
		return
	}
	mediaItemID, ok := parseMediaItemID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.LinkMedia(r.Context(), artefactID, mediaItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error linking media: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("artefact %d not found", artefactID))
		return
	}
	writeJSON(w, resp)
}

// ── UnlinkMedia ───────────────────────────────────────────────────────────────

func (h *ArtefactHandler) UnlinkMedia(w http.ResponseWriter, r *http.Request) {
	artefactID, ok := parseArtefactID(w, r)
	if !ok {
		return
	}
	mediaItemID, ok := parseMediaItemID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.UnlinkMedia(r.Context(), artefactID, mediaItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error removing media: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("artefact %d not found", artefactID))
		return
	}
	writeJSON(w, resp)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseArtefactID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "artefact_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "artefact_id must be an integer")
		return 0, false
	}
	return id, true
}

func parseMediaItemID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "media_item_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "media_item_id must be an integer")
		return 0, false
	}
	return id, true
}

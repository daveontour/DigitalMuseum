package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ImageHandler handles all /images/*, /getLocations, and /facebook/albums/* read endpoints.
type ImageHandler struct {
	svc *service.ImageService
}

// NewImageHandler creates an ImageHandler.
func NewImageHandler(svc *service.ImageService) *ImageHandler {
	return &ImageHandler{svc: svc}
}

// RegisterRoutes mounts all image routes onto r.
func (h *ImageHandler) RegisterRoutes(r chi.Router) {
	// Specific sub-paths must be registered before parameterised {image_id} routes.
	r.Get("/images/search", h.Search)
	r.Get("/images/years", h.GetYears)
	r.Get("/images/tags", h.GetTags)
	r.Put("/images/bulk-update", h.BulkUpdate)
	r.Delete("/images/bulk-delete", h.BulkDelete)
	r.Delete("/images", h.DeleteByRange)

	r.Get("/images/{image_id}/metadata", h.GetMetadata)
	r.Get("/images/{image_id}/thumbnail", h.GetThumbnail)
	r.Get("/images/{image_id}", h.GetContent)
	r.Put("/images/{image_id}", h.UpdateMetadata)
	r.Delete("/images/{image_id}", h.Delete)

	// Location map endpoint
	r.Get("/getLocations", h.GetLocations)

	// Facebook album and posts read endpoints
	r.Get("/facebook/albums", h.GetFacebookAlbums)
	r.Get("/facebook/posts", h.GetFacebookPosts)
	// /facebook/posts/media/{media_id} must come before /facebook/posts/{post_id}/media
	r.Get("/facebook/posts/media/{media_id}", h.GetFacebookPostMediaContent)
	r.Get("/facebook/posts/{post_id}/media", h.GetFacebookPostMedia)
	// Note: /facebook/albums/images/{id} must come before /facebook/albums/{album_id}/images
	// to avoid chi treating "images" as an album_id value.
	r.Get("/facebook/albums/images/{image_id}", h.GetAlbumImageContent)
	r.Get("/facebook/albums/{album_id}/images", h.GetAlbumImages)
	r.Get("/facebook/places", h.GetFacebookPlaces)
}

// ── /images/search ────────────────────────────────────────────────────────────

func (h *ImageHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := model.ImageSearchParams{}

	if v := q.Get("title"); v != "" {
		p.Title = &v
	}
	if v := q.Get("description"); v != "" {
		p.Description = &v
	}
	if v := q.Get("author"); v != "" {
		p.Author = &v
	}
	if v := q.Get("tags"); v != "" {
		p.Tags = &v
	}
	if v := q.Get("categories"); v != "" {
		p.Categories = &v
	}
	if v := q.Get("source"); v != "" {
		p.Source = &v
	}
	if v := q.Get("source_reference"); v != "" {
		p.SourceReference = &v
	}
	if v := q.Get("media_type"); v != "" {
		p.MediaType = &v
	}
	if v := q.Get("region"); v != "" {
		p.Region = &v
	}
	if v := q.Get("year"); v != "" {
		yr, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "year must be an integer")
			return
		}
		p.Year = &yr
	}
	if v := q.Get("month"); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m < 1 || m > 12 {
			writeError(w, http.StatusBadRequest, "month must be an integer between 1 and 12")
			return
		}
		p.Month = &m
	}
	if v := q.Get("rating"); v != "" {
		rt, err := strconv.Atoi(v)
		if err != nil || rt < 1 || rt > 5 {
			writeError(w, http.StatusBadRequest, "rating must be an integer between 1 and 5")
			return
		}
		p.Rating = &rt
	}
	if v := q.Get("rating_min"); v != "" {
		rt, err := strconv.Atoi(v)
		if err != nil || rt < 1 || rt > 5 {
			writeError(w, http.StatusBadRequest, "rating_min must be an integer between 1 and 5")
			return
		}
		p.RatingMin = &rt
	}
	if v := q.Get("rating_max"); v != "" {
		rt, err := strconv.Atoi(v)
		if err != nil || rt < 1 || rt > 5 {
			writeError(w, http.StatusBadRequest, "rating_max must be an integer between 1 and 5")
			return
		}
		p.RatingMax = &rt
	}
	if v := q.Get("has_gps"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "has_gps must be true or false")
			return
		}
		p.HasGPS = &b
	}
	if v := q.Get("available_for_task"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "available_for_task must be true or false")
			return
		}
		p.AvailableForTask = &b
	}
	if v := q.Get("processed"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "processed must be true or false")
			return
		}
		p.Processed = &b
	}

	result, err := h.svc.Search(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error searching images: %s", err))
		return
	}
	writeJSON(w, result)
}

// ── /images/years ─────────────────────────────────────────────────────────────

func (h *ImageHandler) GetYears(w http.ResponseWriter, r *http.Request) {
	years, err := h.svc.GetDistinctYears(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving distinct years: %s", err))
		return
	}
	if years == nil {
		years = []int{}
	}
	writeJSON(w, map[string]any{"years": years})
}

// ── /images/tags ──────────────────────────────────────────────────────────────

func (h *ImageHandler) GetTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.svc.GetDistinctTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving distinct tags: %s", err))
		return
	}
	if tags == nil {
		tags = []string{}
	}
	writeJSON(w, map[string]any{"tags": tags})
}

// ── /facebook/places ──────────────────────────────────────────────────────────

func (h *ImageHandler) GetFacebookPlaces(w http.ResponseWriter, r *http.Request) {
	places, err := h.svc.GetFacebookPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving Facebook places: %s", err))
		return
	}
	if places == nil {
		places = []model.FacebookPlaceItem{}
	}
	writeJSON(w, map[string]any{"places": places})
}

// ── /getLocations ─────────────────────────────────────────────────────────────

func (h *ImageHandler) GetLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := h.svc.GetLocations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving locations: %s", err))
		return
	}
	if locations == nil {
		locations = []model.LocationItem{}
	}
	writeJSON(w, map[string]any{"locations": locations})
}

// ── /images/{image_id} ────────────────────────────────────────────────────────

// GetContent handles GET /images/{image_id}
// Query params:
//   - type: "blob" (default) | "metadata"
//   - preview: bool (default false) — returns thumbnail if true
//   - convert_heic_to_jpg: bool (default true) — accepted but HEIC conversion is not
//     implemented in Go; the image is returned as-is with its original content type.
func (h *ImageHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}

	idType := r.URL.Query().Get("type")
	if idType == "" {
		idType = "blob"
	}
	if idType != "blob" && idType != "metadata" {
		writeError(w, http.StatusBadRequest, `type must be "blob" or "metadata"`)
		return
	}

	preview := false
	if v := r.URL.Query().Get("preview"); v != "" {
		var err error
		preview, err = strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "preview must be true or false")
			return
		}
	}

	h.serveImageContent(w, r, id, idType, preview)
}

// GetThumbnail handles GET /images/{image_id}/thumbnail — convenience alias for ?preview=true.
func (h *ImageHandler) GetThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}
	h.serveImageContent(w, r, id, "metadata", true)
}

// GetMetadata handles GET /images/{image_id}/metadata
func (h *ImageHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetMetadata(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving image metadata: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("image with ID %d not found", id))
		return
	}
	writeJSON(w, resp)
}

// ── /facebook/albums/* and /facebook/posts ─────────────────────────────────────

func (h *ImageHandler) GetFacebookPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := repository.GetFacebookPostsParams{
		Page:     1,
		PageSize: 50,
	}
	if v := q.Get("search"); v != "" {
		p.Search = v
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			p.Page = n
		}
	}
	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			if n > 200 {
				n = 200
			}
			p.PageSize = n
		}
	}
	if v := q.Get("post_ids"); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				p.PostIDs = append(p.PostIDs, n)
			}
		}
	}

	resp, err := h.svc.GetFacebookPosts(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving Facebook posts: %s", err))
		return
	}
	writeJSON(w, resp)
}

func (h *ImageHandler) GetFacebookPostMediaContent(w http.ResponseWriter, r *http.Request) {
	mediaID, ok := parseImageID(w, r, "media_id")
	if !ok {
		return
	}

	content, err := h.svc.GetPostMediaContent(r.Context(), mediaID)
	if err != nil {
		if strings.Contains(err.Error(), "no image data") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("media item %d has no image data", mediaID))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving media: %s", err))
		return
	}
	if content == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("media item %d not found or not linked to a post", mediaID))
		return
	}

	serveBinaryContent(w, content)
}

func (h *ImageHandler) GetFacebookPostMedia(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseImageID(w, r, "post_id")
	if !ok {
		return
	}

	items, err := h.svc.GetPostMedia(r.Context(), postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving post media: %s", err))
		return
	}
	writeJSON(w, items)
}

func (h *ImageHandler) GetFacebookAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := h.svc.GetFacebookAlbums(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving albums: %s", err))
		return
	}
	writeJSON(w, albums)
}

func (h *ImageHandler) GetAlbumImages(w http.ResponseWriter, r *http.Request) {
	albumID, ok := parseImageID(w, r, "album_id")
	if !ok {
		return
	}

	images, err := h.svc.GetAlbumImages(r.Context(), albumID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving album images: %s", err))
		return
	}
	writeJSON(w, images)
}

func (h *ImageHandler) GetAlbumImageContent(w http.ResponseWriter, r *http.Request) {
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}

	content, err := h.svc.GetAlbumImageContent(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no image data") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("image with ID %d has no image data", id))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving image: %s", err))
		return
	}
	if content == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("image with ID %d not found or not linked to an album", id))
		return
	}

	serveBinaryContent(w, content)
}

// ── Write / delete ────────────────────────────────────────────────────────────

func (h *ImageHandler) BulkUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageIDs []int64 `json:"image_ids"`
		Tags     string  `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, errs := h.svc.BulkUpdateTags(r.Context(), req.ImageIDs, req.Tags)
	resp := map[string]any{
		"message":       fmt.Sprintf("Updated %d image(s)", updated),
		"updated_count": updated,
	}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	writeJSON(w, resp)
}

func (h *ImageHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageIDs []int64 `json:"image_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	deleted, errs := h.svc.BulkDeleteImages(r.Context(), req.ImageIDs)
	resp := map[string]any{
		"message":       fmt.Sprintf("Deleted %d image(s)", deleted),
		"deleted_count": deleted,
	}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	writeJSON(w, resp)
}

func (h *ImageHandler) DeleteByRange(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	all := strings.ToLower(q.Get("all")) == "true" || q.Get("all") == "1"
	var startID, endID *int64
	if s := q.Get("start_id"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "start_id must be an integer")
			return
		}
		startID = &id
	}
	if s := q.Get("end_id"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "end_id must be an integer")
			return
		}
		endID = &id
	}
	deleted, err := h.svc.DeleteByIDRange(r.Context(), all, startID, endID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if deleted == 0 && all {
		writeError(w, http.StatusNotFound, "No images found to delete")
		return
	}
	writeJSON(w, map[string]any{
		"message":       fmt.Sprintf("Successfully deleted %d image(s)", deleted),
		"deleted_count": deleted,
	})
}

func (h *ImageHandler) UpdateMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var description, tags *string
	var rating *int
	if v, ok := req["description"].(string); ok {
		description = &v
	}
	if v, ok := req["tags"].(string); ok {
		tags = &v
	}
	if v, ok := req["rating"].(float64); ok {
		rt := int(v)
		rating = &rt
	}
	ok2, err := h.svc.UpdateMetadata(r.Context(), id, description, tags, rating)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok2 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Image with ID %d not found", id))
		return
	}
	writeJSON(w, map[string]any{
		"message":  fmt.Sprintf("Image %d updated successfully", id),
		"image_id": id,
		"updated_fields": map[string]bool{
			"description": description != nil,
			"tags":        tags != nil,
			"rating":      rating != nil,
		},
	})
}

func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}
	deleted, err := h.svc.DeleteByMetadataID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Image with metadata ID %d not found", id))
		return
	}
	writeJSON(w, map[string]any{
		"message":  fmt.Sprintf("Image %d deleted successfully", id),
		"image_id": id,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (h *ImageHandler) serveImageContent(w http.ResponseWriter, r *http.Request, id int64, idType string, preview bool) {
	content, err := h.svc.GetImageContent(r.Context(), id, idType, preview)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no thumbnail") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("image with ID %d has no thumbnail available", id))
			return
		}
		if strings.Contains(msg, "no image data") || strings.Contains(msg, "not found") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("image with ID %d not found or has no data", id))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving image: %s", err))
		return
	}
	if content == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("image with ID %d not found", id))
		return
	}
	serveBinaryContent(w, content)
}

func serveBinaryContent(w http.ResponseWriter, c *model.ImageContent) {
	w.Header().Set("Content-Type", c.ContentType)
	if c.Filename != "" {
		safe := strings.ReplaceAll(c.Filename, `"`, `\"`)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safe))
	}
	_, _ = w.Write(c.Data)
}

func parseImageID(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	raw := chi.URLParam(r, param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, param+" must be an integer")
		return 0, false
	}
	return id, true
}

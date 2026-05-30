package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/daveontour/aimuseum/internal/sqlutil"
	"github.com/go-chi/chi/v5"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

var imageAIClassificationJob = importer.NewImportJob("Image AI classification", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"total": 0, "processed": 0,
})

var imageTagEmbeddingJob = importer.NewImportJob("Image tag embeddings", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"total": 0, "processed": 0,
})

var facebookPostEmbeddingJob = importer.NewImportJob("Facebook post text embeddings", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"total": 0, "processed": 0,
})

var facebookAlbumEmbeddingJob = importer.NewImportJob("Facebook album description embeddings", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"total": 0, "processed": 0,
})

// ctxKeyRunpodAPI carries a resolved RunPod bearer token for image classification workers.
type ctxKeyRunpodAPI struct{}

func withRunpodAPIKey(ctx context.Context, apiKey string) context.Context {
	return context.WithValue(ctx, ctxKeyRunpodAPI{}, strings.TrimSpace(apiKey))
}

func runpodAPIKeyFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRunpodAPI{}).(string)
	return v
}

// ctxKeyRunpodEndpointID carries a resolved RunPod endpoint ID for image classification workers.
type ctxKeyRunpodEndpointID struct{}

func withRunpodEndpointID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRunpodEndpointID{}, strings.TrimSpace(id))
}

func runpodEndpointIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRunpodEndpointID{}).(string)
	return v
}

func effectiveRunpodEndpointID(ctx context.Context, pool *sql.DB, uid int64) string {
	if pool != nil && uid != 0 {
		ur := repository.NewUserRepo(pool)
		stored, err := ur.GetUserLLMStored(ctx, uid)
		if err == nil && stored != nil && strings.TrimSpace(stored.RunpodEndpointID) != "" {
			return strings.TrimSpace(stored.RunpodEndpointID)
		}
	}
	return strings.TrimSpace(os.Getenv("RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID"))
}

func effectiveRunpodWorkers(ctx context.Context, pool *sql.DB, uid int64) int {
	if pool != nil && uid != 0 {
		ur := repository.NewUserRepo(pool)
		stored, err := ur.GetUserLLMStored(ctx, uid)
		if err == nil && stored != nil && stored.RunpodWorkers > 0 {
			return clampRunPodImageAIWorkers(stored.RunpodWorkers)
		}
	}
	return imageAIClassificationWorkerCountFromEnv()
}

func effectiveRunpodAPIKey(ctx context.Context, pool *sql.DB, uid int64) string {
	env := strings.TrimSpace(os.Getenv("RUNPOD_API_KEY"))
	if pool == nil || uid == 0 {
		return env
	}
	ur := repository.NewUserRepo(pool)
	stored, err := ur.GetUserLLMStored(ctx, uid)
	if err != nil || stored == nil {
		return env
	}
	if strings.TrimSpace(stored.RunpodAPIKey) != "" {
		return strings.TrimSpace(stored.RunpodAPIKey)
	}
	if !stored.AllowServerLLMKeys {
		return ""
	}
	return env
}

// ImageHandler handles all /images/*, /getLocations, and /facebook/albums/* read endpoints.
type ImageHandler struct {
	svc          *service.ImageService
	sessionStore *keystore.SessionMasterStore
	pool         *sql.DB
	embeddingSvc *service.EmbeddingService
}

// NewImageHandler creates an ImageHandler. embeddingSvc may be nil (similarity search and tag embed jobs unavailable).
func NewImageHandler(svc *service.ImageService, sessionStore *keystore.SessionMasterStore, pool *sql.DB, embeddingSvc *service.EmbeddingService) *ImageHandler {
	return &ImageHandler{svc: svc, sessionStore: sessionStore, pool: pool, embeddingSvc: embeddingSvc}
}

// RegisterRoutes mounts all image routes onto r.
func (h *ImageHandler) RegisterRoutes(r chi.Router) {
	// Specific sub-paths must be registered before parameterised {image_id} routes.
	r.Post("/image/ai-classification", h.ImageAIClassificationStart)
	r.Post("/image/ai-classification/missing-gemma-classified", h.ImageAIClassificationGemmaUnclassifiedStart)
	r.Get("/image/ai-classification/stream", h.ImageAIClassificationStream)
	r.Post("/image/ai-classification/cancel", h.ImageAIClassificationCancel)
	r.Get("/image/ai-classification/status", h.ImageAIClassificationStatus)

	r.Post("/images/tag-embeddings/backfill", h.ImageTagEmbeddingsBackfillStart)
	r.Get("/images/tag-embeddings/backfill/stream", h.ImageTagEmbeddingsBackfillStream)
	r.Post("/images/tag-embeddings/backfill/cancel", h.ImageTagEmbeddingsBackfillCancel)
	r.Get("/images/tag-embeddings/backfill/status", h.ImageTagEmbeddingsBackfillStatus)
	r.Post("/images/similar-by-tags", h.SimilarByTagsSearch)
	r.Post("/facebook/posts/embeddings/backfill", h.FacebookPostEmbeddingsBackfillStart)
	r.Get("/facebook/posts/embeddings/backfill/stream", h.FacebookPostEmbeddingsBackfillStream)
	r.Post("/facebook/posts/embeddings/backfill/cancel", h.FacebookPostEmbeddingsBackfillCancel)
	r.Get("/facebook/posts/embeddings/backfill/status", h.FacebookPostEmbeddingsBackfillStatus)
	r.Post("/facebook/posts/similar-by-text", h.SimilarFacebookPostsByText)
	r.Post("/facebook/albums/embeddings/backfill", h.FacebookAlbumEmbeddingsBackfillStart)
	r.Get("/facebook/albums/embeddings/backfill/stream", h.FacebookAlbumEmbeddingsBackfillStream)
	r.Post("/facebook/albums/embeddings/backfill/cancel", h.FacebookAlbumEmbeddingsBackfillCancel)
	r.Get("/facebook/albums/embeddings/backfill/status", h.FacebookAlbumEmbeddingsBackfillStatus)
	r.Post("/facebook/albums/similar-by-description", h.SimilarFacebookAlbumsByDescription)

	r.Get("/images/search", h.Search)
	r.Get("/images/years", h.GetYears)
	r.Get("/images/tags", h.GetTags)
	r.Get("/images/gps-count-by-source", h.GetGPSCountBySource)
	r.Get("/images/gps-regions", h.GetGPSRegions)
	r.Get("/images/regions", h.GetImageRegions)
	r.Post("/images/locations/nearby", h.GetNearbyLocations)
	r.Put("/images/bulk-update", h.BulkUpdate)
	r.Delete("/images/bulk-delete", h.BulkDelete)
	r.Delete("/images", h.DeleteByRange)

	r.Get("/images/{image_id}/metadata", h.GetMetadata)
	r.Get("/images/{image_id}/thumbnail", h.GetThumbnail)
	r.Get("/images/{image_id}", h.GetContent)
	r.Put("/images/{image_id}", h.UpdateMetadata)
	r.Delete("/images/{image_id}", h.Delete)

	// Location map endpoints
	r.Get("/getLocations", h.GetLocations)
	r.Post("/getLocations/random", h.GetRandomLocations)

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
	if v := q.Get("has_thumbnail"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "has_thumbnail must be true or false")
			return
		}
		p.HasThumbnail = &b
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

// ── /images/gps-count-by-source ───────────────────────────────────────────────

func (h *ImageHandler) GetGPSCountBySource(w http.ResponseWriter, r *http.Request) {
	counts, err := h.svc.CountGPSBySource(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error counting GPS images by source: %s", err))
		return
	}
	if counts == nil {
		counts = []model.GPSCountBySource{}
	}
	var total int64
	for _, row := range counts {
		total += row.Count
	}
	writeJSON(w, map[string]any{"by_source": counts, "total": total})
}

// ── /images/gps-regions ───────────────────────────────────────────────────────

func (h *ImageHandler) GetGPSRegions(w http.ResponseWriter, r *http.Request) {
	regions, err := h.svc.CountGPSByRegion(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error counting GPS images by region: %s", err))
		return
	}
	if regions == nil {
		regions = []model.GPSRegionCount{}
	}
	attachRegionLabelsGPS(regions)
	writeJSON(w, map[string]any{"regions": regions})
}

// ── /images/regions ───────────────────────────────────────────────────────────

func (h *ImageHandler) GetImageRegions(w http.ResponseWriter, r *http.Request) {
	regions, err := h.svc.CountImagesByRegion(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error counting images by region: %s", err))
		return
	}
	if regions == nil {
		regions = []model.ImageRegionCount{}
	}
	attachRegionLabelsImages(regions)
	writeJSON(w, map[string]any{"regions": regions})
}

func attachRegionLabelsGPS(rows []model.GPSRegionCount) {
	for i := range rows {
		rows[i].Label = georegion.Label(rows[i].Region)
	}
}

func attachRegionLabelsImages(rows []model.ImageRegionCount) {
	for i := range rows {
		rows[i].Label = georegion.Label(rows[i].Region)
	}
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

// ── /getLocations/random ──────────────────────────────────────────────────────

func (h *ImageHandler) GetRandomLocations(w http.ResponseWriter, r *http.Request) {
	var req model.RandomLocationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}

	locations, err := h.svc.GetRandomLocationsByCategories(r.Context(), req.Categories, strings.TrimSpace(req.Region), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving random locations: %s", err))
		return
	}
	if locations == nil {
		locations = []model.LocationItem{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"locations": locations})
}

// ── /images/locations/nearby ──────────────────────────────────────────────────

func (h *ImageHandler) GetNearbyLocations(w http.ResponseWriter, r *http.Request) {
	var req model.NearbyLocationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.RadiusKm <= 0 || req.RadiusKm > 500 {
		writeError(w, http.StatusBadRequest, "radius_km must be greater than 0 and at most 500")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}

	locations, err := h.svc.GetNearbyLocations(r.Context(), req.Latitude, req.Longitude, req.RadiusKm, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving nearby locations: %s", err))
		return
	}
	if locations == nil {
		locations = []model.NearbyLocationItem{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{
		"locations": locations,
		"center": map[string]float64{
			"latitude":  req.Latitude,
			"longitude": req.Longitude,
		},
		"radius_km": req.RadiusKm,
		"total":     len(locations),
	})
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
	if items == nil {
		items = []model.FacebookPostMediaItem{}
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
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
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

// ImageAIClassificationStart starts a background stub job that processes the given image IDs.
// POST /image/ai-classification  body: { "ids": [1,2,3], "workers": 4 }
// Optional "workers" (0–32) is RunPod worker count; one additional worker always runs local Ollama as primary.
// Overrides IMAGE_AI_CLASSIFICATION_WORKERS (RunPod count) when set.
func (h *ImageHandler) ImageAIClassificationStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	var req struct {
		IDs     []int64 `json:"ids"`
		Workers *int    `json:"workers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}
	if err := imageAIClassificationJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	uid := appctx.UserIDFromCtx(r.Context())
	idsCopy := append([]int64(nil), req.IDs...)

	imageAIClassificationJob.Start()
	imageAIClassificationJob.UpdateState(map[string]any{
		"status":        "in_progress",
		"status_line":   fmt.Sprintf("Starting AI classification for %d image(s)...", len(idsCopy)),
		"total":         len(idsCopy),
		"processed":     0,
		"classified":    0,
		"errors":        0,
		"error_message": nil,
	})
	imageAIClassificationJob.Broadcast("status", map[string]any{"status_line": fmt.Sprintf("Starting AI classification for %d image(s)...", len(idsCopy))})

	workerArg := 0
	if req.Workers != nil {
		workerArg = clampRunPodImageAIWorkers(*req.Workers)
	}
	go runImageAIClassificationStub(h.svc, imageAIClassificationJob, h.pool, uid, idsCopy, workerArg)

	writeJSON(w, map[string]any{"message": "Image AI classification started", "status": "started", "count": len(idsCopy)})
}

// ImageAIClassificationGemmaUnclassifiedStart starts classification for image rows
// flagged require_classification=true.
func (h *ImageHandler) ImageAIClassificationGemmaUnclassifiedStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if err := imageAIClassificationJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := h.svc.ListImageIDsRequireClassification(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build classification list: %v", err))
		return
	}
	uid := appctx.UserIDFromCtx(r.Context())

	imageAIClassificationJob.Start()
	imageAIClassificationJob.UpdateState(map[string]any{
		"status":        "in_progress",
		"status_line":   fmt.Sprintf("Starting AI classification for %d queued image(s)...", len(ids)),
		"total":         len(ids),
		"processed":     0,
		"classified":    0,
		"errors":        0,
		"error_message": nil,
	})
	imageAIClassificationJob.Broadcast("status", map[string]any{
		"status_line": fmt.Sprintf("Starting AI classification for %d queued image(s)...", len(ids)),
	})

	go runImageAIClassificationStub(h.svc, imageAIClassificationJob, h.pool, uid, append([]int64(nil), ids...), 0)

	writeJSON(w, map[string]any{
		"message": "Image AI classification job started for queued images",
		"status":  "started",
		"count":   len(ids),
	})
}

func (h *ImageHandler) ImageAIClassificationStream(w http.ResponseWriter, r *http.Request) {
	imageAIClassificationJob.ServeSSE(w, r)
}

func (h *ImageHandler) ImageAIClassificationCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	writeJSON(w, imageAIClassificationJob.Cancel())
}

func (h *ImageHandler) ImageAIClassificationStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imageAIClassificationJob.Status())
}

// normalizePutImageTags maps JSON "tags" (string or array of strings per API spec) to a single comma-separated value.
func normalizePutImageTags(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case string:
		return &t, nil
	case []any:
		parts := make([]string, 0, len(t))
		for i, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("tags: element %d must be a string", i)
			}
			parts = append(parts, s)
		}
		out := strings.Join(parts, ", ")
		return &out, nil
	default:
		return nil, fmt.Errorf("tags: expected string or array of strings, got %T", v)
	}
}

// normalizePutImageRating maps JSON "rating" (number or numeric string) to int for media_items.rating.
func normalizePutImageRating(v any) (*int, error) {
	if v == nil {
		return nil, nil
	}
	var n int
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) {
			return nil, fmt.Errorf("rating must be a whole number between 1 and 5")
		}
		n = int(t)
	case json.Number:
		i64, err := t.Int64()
		if err != nil {
			f, ferr := t.Float64()
			if ferr != nil {
				return nil, fmt.Errorf("rating: invalid number")
			}
			if f != math.Trunc(f) {
				return nil, fmt.Errorf("rating must be a whole number between 1 and 5")
			}
			n = int(f)
		} else {
			n = int(i64)
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return nil, fmt.Errorf("rating must be an integer between 1 and 5")
		}
		n = i
	case int:
		n = t
	case int64:
		n = int(t)
	default:
		return nil, fmt.Errorf("rating: unsupported type %T", v)
	}
	if n < 1 || n > 5 {
		return nil, fmt.Errorf("rating must be between 1 and 5")
	}
	return &n, nil
}

func normalizeImageMIME(contentType string) string {
	ct := strings.TrimSpace(contentType)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return strings.ToLower(ct)
}

// convertImageBytesToJPEGIfNeeded returns JPEG bytes. If the input is already a valid JPEG, returns data unchanged.
func convertImageBytesToJPEGIfNeeded(data []byte, contentType string) ([]byte, error) {
	ct := normalizeImageMIME(contentType)
	if ct == "image/jpeg" || ct == "image/jpg" {
		if _, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
			return data, nil
		}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

const (
	imageAIClassificationDefaultWorkers = 4
	imageAIClassificationMaxWorkers     = 8
)

// clampRunPodImageAIWorkers bounds RunPod-side parallelism (0 = RunPod workers disabled; local-only pool still runs).
func clampRunPodImageAIWorkers(n int) int {
	if n < 0 {
		return 0
	}
	if n > imageAIClassificationMaxWorkers {
		return imageAIClassificationMaxWorkers
	}
	return n
}

// imageAIClassificationWorkerCountFromEnv returns IMAGE_AI_CLASSIFICATION_WORKERS (RunPod worker count) or the default (clamped).
func imageAIClassificationWorkerCountFromEnv() int {
	s := strings.TrimSpace(os.Getenv("IMAGE_AI_CLASSIFICATION_WORKERS"))
	if s == "" {
		return imageAIClassificationDefaultWorkers
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return imageAIClassificationDefaultWorkers
	}
	return clampRunPodImageAIWorkers(n)
}

const (
	imageAIWorkerPoolRunPod = "RunPod"
	imageAIWorkerPoolLocal  = "Local"
)

// imageAIClassifyOutcome is the result of classifying one image (workers send this; the coordinator broadcasts).
type imageAIClassifyOutcome struct {
	id          int64
	classified  bool
	keywordTags string
	errMsg      string
	workerPool  string // owning goroutine pool: RunPod or Local
	workerNum   int    // 1-based index within that pool
	successVia  string // backend that produced keywords when classified: RunPod or Local; empty on failure
}

// imageAIOutcomeWorkerUIPrefix returns a short tag for status_line / SSE UI (e.g. "[RunPod-2]" or "[RunPod-2->Local]").
func imageAIOutcomeWorkerUIPrefix(o imageAIClassifyOutcome) string {
	if o.workerPool == "" || o.workerNum < 1 {
		return "[worker]"
	}
	tag := fmt.Sprintf("%s-%d", o.workerPool, o.workerNum)
	if o.classified && o.successVia != "" && o.successVia != o.workerPool {
		return fmt.Sprintf("[%s->%s]", tag, o.successVia)
	}
	return "[" + tag + "]"
}

// classifyImageAIOne loads an image, runs Ollama keyword classification, and updates tags. It does not touch job state.
func classifyImageAIOne(ctx context.Context, svc *service.ImageService, id int64) imageAIClassifyOutcome {
	content, err := svc.GetImageContent(ctx, id, "metadata", false)
	if err != nil || content == nil || len(content.Data) == 0 {
		msg := fmt.Sprintf("load image %d: %v", id, err)
		if content == nil && err == nil {
			msg = fmt.Sprintf("load image %d: not found", id)
		}
		return imageAIClassifyOutcome{id: id, errMsg: msg}
	}

	jpegBytes, err := convertImageBytesToJPEGIfNeeded(content.Data, content.ContentType)
	if err != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("jpeg convert image %d: %v", id, err)}
	}

	encodedB64 := base64.StdEncoding.EncodeToString(jpegBytes)
	keywordTags, llmErr := classifyImageKeywordsWithOllama(ctx, encodedB64)
	if llmErr != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("classify image %d: %v", id, llmErr)}
	}
	if strings.TrimSpace(keywordTags) == "" {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("classify image %d: model returned no keywords", id)}
	}

	tagsForUpdate := keywordTags
	ok, mergeErr := svc.UpdateTagsMerge(ctx, id, tagsForUpdate)
	if mergeErr != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("tag update image %d: %v", id, mergeErr)}
	}
	if !ok {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("tag update image %d failed", id)}
	}
	svc.SyncTagEmbedding(ctx, id)
	if _, err := svc.SetRequireClassification(ctx, id, false); err != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("clear require_classification image %d: %v", id, err)}
	}

	return imageAIClassifyOutcome{id: id, classified: true, keywordTags: keywordTags}
}

// runPodImageClassifyRunsyncURL returns the RunPod serverless runsync URL.
// Priority: context-stored endpoint ID (from user settings) → RUNPOD_IMAGE_CLASSIFY_URL env var
// → RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID env var.
func runPodImageClassifyRunsyncURL(ctx context.Context) (string, error) {
	if id := runpodEndpointIDFromCtx(ctx); id != "" {
		return "https://api.runpod.ai/v2/" + id + "/runsync", nil
	}
	raw := strings.TrimSpace(os.Getenv("RUNPOD_IMAGE_CLASSIFY_URL"))
	if raw != "" {
		u := strings.TrimRight(raw, "/")
		if !strings.HasSuffix(u, "/run") && !strings.HasSuffix(u, "/runsync") {
			u += "/runsync"
		}
		return u, nil
	}
	id := strings.TrimSpace(os.Getenv("RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID"))
	if id == "" {
		return "", fmt.Errorf("set RUNPOD_IMAGE_CLASSIFY_URL or RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID (or save an Endpoint ID in Settings → API Keys)")
	}
	return "https://api.runpod.ai/v2/" + id + "/runsync", nil
}

// extractRunPodAssistantContentForKeywords parses a RunPod runsync JSON body and returns the
// assistant message.content string (expected to be a JSON array of keywords), matching runpod_image_classify.py.
func extractRunPodAssistantContentForKeywords(result map[string]any) (string, error) {
	if result == nil {
		return "", fmt.Errorf("response is not a JSON object")
	}
	if errVal, ok := result["error"]; ok && errVal != nil {
		return "", fmt.Errorf("runpod error: %v", errVal)
	}
	outRaw, ok := result["output"]
	if !ok || outRaw == nil {
		return "", fmt.Errorf("no output in response")
	}
	if outMap, ok := outRaw.(map[string]any); ok {
		if errVal, ok := outMap["error"]; ok && errVal != nil {
			return "", fmt.Errorf("runpod output error: %v", errVal)
		}
		return "", fmt.Errorf("unexpected output shape (object)")
	}
	outList, ok := outRaw.([]any)
	if !ok || len(outList) == 0 {
		return "", fmt.Errorf("unexpected or empty output")
	}
	for i := len(outList) - 1; i >= 0; i-- {
		item, ok := outList[i].(map[string]any)
		if !ok {
			continue
		}
		if errVal, ok := item["error"]; ok && errVal != nil {
			return "", fmt.Errorf("runpod output item error: %v", errVal)
		}
		msg, ok := item["message"].(map[string]any)
		if !ok {
			continue
		}
		c, ok := msg["content"].(string)
		if ok && strings.TrimSpace(c) != "" {
			return strings.TrimSpace(c), nil
		}
	}
	return "", fmt.Errorf("no assistant message.content found in output")
}

// classifyImageKeywordsWithRunPod POSTs base64 JPEG bytes (no data: prefix) to RunPod serverless
// with input key imageClassifyRequest, same contract as internal/handler/runpod_image_classify.py.
func classifyImageKeywordsWithRunPod(ctx context.Context, encodedImageB64 string) (string, error) {
	apiKey := strings.TrimSpace(runpodAPIKeyFromCtx(ctx))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("RUNPOD_API_KEY"))
	}
	if apiKey == "" {
		return "", fmt.Errorf("RunPod API key is not set (set RUNPOD_API_KEY or save a key under Settings → API Keys)")
	}
	url, err := runPodImageClassifyRunsyncURL(ctx)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"input": map[string]any{
			"imageClassifyRequest": encodedImageB64,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal runpod request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build runpod request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call runpod: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read runpod response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("runpod API status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var result map[string]any
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", fmt.Errorf("decode runpod response: %w", err)
		}
	} else {
		result = map[string]any{}
	}
	rawContent, err := extractRunPodAssistantContentForKeywords(result)
	if err != nil {
		return "", err
	}
	keywords, err := parseKeywordJSONArray(rawContent)
	if err != nil {
		return "", fmt.Errorf("parse runpod keyword content: %w", err)
	}
	return strings.Join(keywords, ", "), nil
}

// classifyImageAIOneRunPod is like classifyImageAIOne but sends the JPEG to RunPod (imageClassifyRequest / runsync)
// instead of calling local Ollama. Requires a RunPod bearer token (RUNPOD_API_KEY or per-user key in Settings) and RUNPOD_IMAGE_CLASSIFY_URL or RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID.
func classifyImageAIOneRunPod(ctx context.Context, svc *service.ImageService, id int64) imageAIClassifyOutcome {
	content, err := svc.GetImageContent(ctx, id, "metadata", false)
	if err != nil || content == nil || len(content.Data) == 0 {
		msg := fmt.Sprintf("load image %d: %v", id, err)
		if content == nil && err == nil {
			msg = fmt.Sprintf("load image %d: not found", id)
		}
		return imageAIClassifyOutcome{id: id, errMsg: msg}
	}

	jpegBytes, err := convertImageBytesToJPEGIfNeeded(content.Data, content.ContentType)
	if err != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("jpeg convert image %d: %v", id, err)}
	}

	encodedB64 := base64.StdEncoding.EncodeToString(jpegBytes)
	keywordTags, llmErr := classifyImageKeywordsWithRunPod(ctx, encodedB64)
	if llmErr != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("classify image %d: %v", id, llmErr)}
	}
	if strings.TrimSpace(keywordTags) == "" {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("classify image %d: model returned no keywords", id)}
	}

	tagsForUpdate := keywordTags
	ok, mergeErr := svc.UpdateTagsMerge(ctx, id, tagsForUpdate)
	if mergeErr != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("tag update image %d: %v", id, mergeErr)}
	}
	if !ok {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("tag update image %d failed", id)}
	}
	svc.SyncTagEmbedding(ctx, id)
	if _, err := svc.SetRequireClassification(ctx, id, false); err != nil {
		return imageAIClassifyOutcome{id: id, errMsg: fmt.Sprintf("clear require_classification image %d: %v", id, err)}
	}

	return imageAIClassifyOutcome{id: id, classified: true, keywordTags: keywordTags}
}

// classifyImageAIOneRunPodWithLocalFallback tries RunPod first; on failure, classifies with local Ollama once.
func classifyImageAIOneRunPodWithLocalFallback(ctx context.Context, svc *service.ImageService, id int64, workerPool string, workerNum int) imageAIClassifyOutcome {
	out := classifyImageAIOneRunPod(ctx, svc, id)
	out.workerPool = workerPool
	out.workerNum = workerNum
	if out.classified {
		out.successVia = imageAIWorkerPoolRunPod
		return out
	}
	fb := classifyImageAIOne(ctx, svc, id)
	if fb.classified {
		return imageAIClassifyOutcome{
			id: id, classified: true, keywordTags: fb.keywordTags,
			workerPool: workerPool, workerNum: workerNum,
			successVia: imageAIWorkerPoolLocal,
		}
	}
	out.successVia = ""
	out.errMsg = fmt.Sprintf("%s; fallback (local): %s", out.errMsg, fb.errMsg)
	return out
}

// classifyImageAIOneWithRunPodFallback tries local Ollama first; on failure, classifies via RunPod once.
func classifyImageAIOneWithRunPodFallback(ctx context.Context, svc *service.ImageService, id int64, workerPool string, workerNum int) imageAIClassifyOutcome {
	out := classifyImageAIOne(ctx, svc, id)
	out.workerPool = workerPool
	out.workerNum = workerNum
	if out.classified {
		out.successVia = imageAIWorkerPoolLocal
		return out
	}
	fb := classifyImageAIOneRunPod(ctx, svc, id)
	if fb.classified {
		return imageAIClassifyOutcome{
			id: id, classified: true, keywordTags: fb.keywordTags,
			workerPool: workerPool, workerNum: workerNum,
			successVia: imageAIWorkerPoolRunPod,
		}
	}
	out.successVia = ""
	out.errMsg = fmt.Sprintf("%s; fallback (RunPod): %s", out.errMsg, fb.errMsg)
	return out
}

func runImageAIClassificationStub(svc *service.ImageService, job *importer.ImportJob, pool *sql.DB, uid int64, imageIDs []int64, workers int) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	ctx = withRunpodAPIKey(ctx, effectiveRunpodAPIKey(ctx, pool, uid))
	ctx = withRunpodEndpointID(ctx, effectiveRunpodEndpointID(ctx, pool, uid))
	defer job.Finish()

	var runPodWorkers int
	if workers <= 0 {
		runPodWorkers = effectiveRunpodWorkers(ctx, pool, uid)
	} else {
		runPodWorkers = clampRunPodImageAIWorkers(workers)
	}

	poolSlots := runPodWorkers + 1 // +1 fixed local-primary worker

	total := len(imageIDs)
	classified, errorsCount := 0, 0
	queueStatusLine := fmt.Sprintf("AI classification: %d image(s) queued [Local 1]", total)
	if runPodWorkers > 0 {
		queueStatusLine = fmt.Sprintf("AI classification: %d image(s) queued [RunPod 1-%d] [Local 1]", total, runPodWorkers)
	}
	job.UpdateState(map[string]any{
		"total":       total,
		"processed":   0,
		"classified":  classified,
		"errors":      errorsCount,
		"status":      "in_progress",
		"status_line": queueStatusLine,
	})
	job.Broadcast("progress", job.GetState())

	if total == 0 {
		job.UpdateState(map[string]any{
			"status":      "completed",
			"status_line": "Image AI classification complete: 0 images",
			"processed":   0,
			"classified":  0,
			"errors":      0,
		})
		job.Broadcast("completed", job.GetState())
		return
	}

	workCh := make(chan int64, poolSlots)
	resCh := make(chan imageAIClassifyOutcome, poolSlots)

	var wg sync.WaitGroup
	for w := 1; w <= runPodWorkers; w++ {
		workerNum := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range workCh {
				resCh <- classifyImageAIOneRunPodWithLocalFallback(ctx, svc, id, imageAIWorkerPoolRunPod, workerNum)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for id := range workCh {
			resCh <- classifyImageAIOneWithRunPodFallback(ctx, svc, id, imageAIWorkerPoolLocal, 1)
		}
	}()

	go func() {
		defer close(workCh)
		for _, id := range imageIDs {
			if job.IsCancelled() {
				return
			}
			workCh <- id
		}
	}()

	go func() {
		wg.Wait()
		close(resCh)
	}()

	processed := 0
	for out := range resCh {
		processed++
		wp := imageAIOutcomeWorkerUIPrefix(out)
		if out.classified {
			classified++
			job.UpdateState(map[string]any{
				"processed":   processed,
				"classified":  classified,
				"errors":      errorsCount,
				"status_line": fmt.Sprintf("%s Processed %d/%d (image id %d, %d AI keywords)", wp, processed, total, out.id, countNonEmptyCSV(out.keywordTags)),
			})
		} else {
			errorsCount++
			job.UpdateState(map[string]any{
				"processed":     processed,
				"classified":    classified,
				"errors":        errorsCount,
				"status_line":   fmt.Sprintf("%s Processed %d/%d — %s", wp, processed, total, out.errMsg),
				"error_message": out.errMsg,
			})
		}
		job.Broadcast("progress", job.GetState())
	}

	if job.IsCancelled() {
		job.UpdateState(map[string]any{
			"status":      "cancelled",
			"status_line": "Image AI classification cancelled.",
			"processed":   processed,
			"classified":  classified,
			"errors":      errorsCount,
		})
		job.Broadcast("cancelled", job.GetState())
		return
	}

	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Image AI classification complete: %d classified, %d errors (of %d)", classified, errorsCount, total),
		"processed":   processed,
		"classified":  classified,
		"errors":      errorsCount,
	})
	job.Broadcast("completed", job.GetState())
}

// ── Tag embeddings backfill & similarity search ───────────────────────────────

func (h *ImageHandler) ImageTagEmbeddingsBackfillStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if err := imageTagEmbeddingJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var reqBody struct {
		ReprocessAll bool `json:"reprocess_all"`
	}
	_ = json.NewDecoder(r.Body).Decode(&reqBody)

	uid := appctx.UserIDFromCtx(r.Context())
	imageTagEmbeddingJob.Start()
	modeLine := "only process rows with require_classification=true"
	if reqBody.ReprocessAll {
		modeLine = "reprocess all tagged images"
	}
	imageTagEmbeddingJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting image tag embedding backfill (" + modeLine + ")...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "skipped_unchanged": 0, "errors": 0, "error_message": nil,
		"reprocess_all": reqBody.ReprocessAll,
	})
	imageTagEmbeddingJob.Broadcast("status", map[string]any{"status_line": "Starting image tag embedding backfill (" + modeLine + ")..."})
	go runImageTagEmbeddingBackfill(h.svc, h.pool, imageTagEmbeddingJob, uid, reqBody.ReprocessAll)
	writeJSON(w, map[string]any{"message": "Image tag embedding backfill started", "status": "started", "reprocess_all": reqBody.ReprocessAll})
}

func (h *ImageHandler) ImageTagEmbeddingsBackfillStream(w http.ResponseWriter, r *http.Request) {
	imageTagEmbeddingJob.ServeSSE(w, r)
}

func (h *ImageHandler) ImageTagEmbeddingsBackfillCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	writeJSON(w, imageTagEmbeddingJob.Cancel())
}

func (h *ImageHandler) ImageTagEmbeddingsBackfillStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, imageTagEmbeddingJob.Status())
}

func runImageTagEmbeddingBackfill(svc *service.ImageService, pool *sql.DB, job *importer.ImportJob, uid int64, reprocessAll bool) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	rows, err := svc.ListMediaItemsForTagEmbeddingBackfill(ctx, !reprocessAll)
	if err != nil {
		job.UpdateState(map[string]any{
			"status": "failed", "status_line": fmt.Sprintf("Failed to list images: %v", err),
			"error_message": err.Error(),
		})
		job.Broadcast("failed", job.GetState())
		return
	}
	total := len(rows)
	job.UpdateState(map[string]any{
		"total": total, "processed": 0, "embedded": 0, "skipped": 0, "skipped_unchanged": 0, "errors": 0,
		"status_line":   fmt.Sprintf("Tag embeddings: %d image row(s) with tags", total),
		"reprocess_all": reprocessAll,
	})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, skippedUnchanged, errorsCount := 0, 0, 0, 0
	for i, row := range rows {
		if job.IsCancelled() {
			_ = recordImportControlLastRun(ctx, pool, uid, "image_tag_embeddings", "cancelled", "cancelled")
			job.UpdateState(map[string]any{
				"status": "cancelled", "status_line": "Image tag embedding backfill cancelled.",
				"processed": i, "embedded": embedded, "skipped": skipped, "skipped_unchanged": skippedUnchanged, "errors": errorsCount,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		tagStr := ""
		if row.Tags != nil {
			tagStr = *row.Tags
		}
		norm := service.NormalizeTagsForEmbedding(tagStr)
		if norm == "" {
			skipped++
			job.UpdateState(map[string]any{
				"processed": i + 1, "embedded": embedded, "skipped": skipped, "skipped_unchanged": skippedUnchanged, "errors": errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d (skipped empty tags after normalize)", i+1, total),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		svc.SyncTagEmbedding(ctx, row.ID)
		_, _ = svc.SetRequireClassification(ctx, row.ID, false)
		embedded++
		job.UpdateState(map[string]any{
			"processed": i + 1, "embedded": embedded, "skipped": skipped, "skipped_unchanged": skippedUnchanged, "errors": errorsCount,
			"status_line": fmt.Sprintf("Processed %d/%d (embedded image id %d)", i+1, total, row.ID),
		})
		job.Broadcast("progress", job.GetState())
	}

	if job.IsCancelled() {
		return
	}
	_ = recordImportControlLastRun(ctx, pool, uid, "image_tag_embeddings", "completed", "")
	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Tag embedding backfill complete: %d embedded, %d skipped (empty tags), %d skipped (unchanged), %d errors", embedded, skipped, skippedUnchanged, errorsCount),
		"processed":   total, "embedded": embedded, "skipped": skipped, "skipped_unchanged": skippedUnchanged, "errors": errorsCount,
	})
	job.Broadcast("completed", job.GetState())
}

// jsonSafeVecDistance ensures sqlite-vec distance values marshal to JSON (NaN/±Inf break encoding/json).
func jsonSafeVecDistance(d any) any {
	switch v := d.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
		return v
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return float64(v)
	default:
		return d
	}
}

func similarByTagsResultRow(meta *model.MediaMetadataResponse, distance any) map[string]any {
	if meta == nil {
		return nil
	}
	return map[string]any{
		"id":                 meta.ID,
		"media_blob_id":      meta.MediaBlobID,
		"description":        meta.Description,
		"title":              meta.Title,
		"author":             meta.Author,
		"tags":               meta.Tags,
		"categories":         meta.Categories,
		"notes":              meta.Notes,
		"available_for_task": meta.AvailableForTask,
		"media_type":         meta.MediaType,
		"processed":          meta.Processed,
		"created_at":         meta.CreatedAt,
		"updated_at":         meta.UpdatedAt,
		"year":               meta.Year,
		"month":              meta.Month,
		"latitude":           meta.Latitude,
		"longitude":          meta.Longitude,
		"altitude":           meta.Altitude,
		"rating":             meta.Rating,
		"has_gps":            meta.HasGPS,
		"google_maps_url":    meta.GoogleMapsURL,
		"region":             meta.Region,
		"source":             meta.Source,
		"source_reference":   meta.SourceReference,
		"distance":           jsonSafeVecDistance(distance),
	}
}

// SimilarByTagsSearch POST /images/similar-by-tags — embed normalized query locally and return nearest images for this user.
func (h *ImageHandler) SimilarByTagsSearch(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var req struct {
		Text string `json:"text"`
		N    int    `json:"n"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	norm := service.NormalizeTagsForEmbedding(req.Text)
	if norm == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.N <= 0 {
		req.N = 25
	}
	if req.N > 50 {
		req.N = 50
	}

	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	vec, err := h.embeddingSvc.EmbedText(ctx, norm)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("embedding failed: %v", err))
		return
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("serialize embedding: %v", err))
		return
	}

	kCandidate := req.N * 25
	if kCandidate < 80 {
		kCandidate = 80
	}
	if kCandidate > 500 {
		kCandidate = 500
	}

	rows, err := h.pool.QueryContext(ctx, `
		SELECT rowid, int_ids, distance
		FROM media_tag_embeddings
		WHERE embedding MATCH ? AND k = ?
		ORDER BY distance ASC
	`, vecBlob, kCandidate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}

	type cand struct {
		id       int64
		distance any
	}
	var candidates []cand
	for rows.Next() {
		var rowID int64
		var intIDs string
		var distance any
		if err := rows.Scan(&rowID, &intIDs, &distance); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan: %v", err))
			return
		}
		candidates = append(candidates, cand{id: rowID, distance: distance})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate: %v", err))
		return
	}
	_ = rows.Close()

	keywords := service.KeywordsForTagSearch(req.Text)
	seenID := make(map[int64]struct{})
	results := make([]map[string]any, 0, req.N)
	for _, c := range candidates {
		if len(results) >= req.N {
			break
		}
		var own int
		if scanErr := h.pool.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM media_items WHERE id = ? AND COALESCE(user_id, 0) = ? AND media_type LIKE 'image/%'`,
			c.id, uid,
		).Scan(&own); scanErr != nil || own == 0 {
			continue
		}
		meta, err := h.svc.GetMetadata(ctx, c.id)
		if err != nil || meta == nil {
			continue
		}
		if _, dup := seenID[meta.ID]; dup {
			continue
		}
		seenID[meta.ID] = struct{}{}
		results = append(results, similarByTagsResultRow(meta, c.distance))
	}

	if len(results) < req.N && len(keywords) > 0 {
		orParts := make([]string, len(keywords))
		args := make([]any, 0, len(keywords)+2)
		args = append(args, uid)
		for i, kw := range keywords {
			orParts[i] = "LOWER(COALESCE(tags, '')) LIKE ?"
			args = append(args, "%"+kw+"%")
		}
		kwLimit := req.N * 15
		if kwLimit < 50 {
			kwLimit = 50
		}
		if kwLimit > 300 {
			kwLimit = 300
		}
		args = append(args, kwLimit)
		q := fmt.Sprintf(`
			SELECT id FROM media_items
			WHERE COALESCE(user_id, 0) = ?
			  AND media_type LIKE 'image/%%'
			  AND tags IS NOT NULL AND TRIM(tags) != ''
			  AND (%s)
			ORDER BY updated_at DESC
			LIMIT ?`, strings.Join(orParts, " OR "))
		kwRows, err := h.pool.QueryContext(ctx, q, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("tag keyword search failed: %v", err))
			return
		}
		// Drain IDs before GetMetadata: SQLite allows only one active statement per
		// connection; nested QueryRowContext inside kwRows.Next() can deadlock.
		var kwIDs []int64
		for kwRows.Next() {
			var kid int64
			if err := kwRows.Scan(&kid); err != nil {
				_ = kwRows.Close()
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("tag keyword scan: %v", err))
				return
			}
			kwIDs = append(kwIDs, kid)
		}
		if err := kwRows.Err(); err != nil {
			_ = kwRows.Close()
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("tag keyword iterate: %v", err))
			return
		}
		_ = kwRows.Close()

		for _, kid := range kwIDs {
			if len(results) >= req.N {
				break
			}
			if _, dup := seenID[kid]; dup {
				continue
			}
			meta, err := h.svc.GetMetadata(ctx, kid)
			if err != nil || meta == nil {
				continue
			}
			seenID[kid] = struct{}{}
			results = append(results, similarByTagsResultRow(meta, nil))
		}
	}

	writeJSON(w, map[string]any{"results": results, "count": len(results), "query_normalized": norm})
}

func normalizeFreeTextForEmbedding(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func shortTextEmbeddingSignature(norm string) string {
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

func vecKCandidate(n int) int {
	k := n * 25
	if k < 80 {
		return 80
	}
	if k > 500 {
		return 500
	}
	return k
}

func (h *ImageHandler) FacebookPostEmbeddingsBackfillStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if err := facebookPostEmbeddingJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var reqBody struct {
		ReprocessAll bool `json:"reprocess_all"`
	}
	_ = json.NewDecoder(r.Body).Decode(&reqBody)
	uid := appctx.UserIDFromCtx(r.Context())
	facebookPostEmbeddingJob.Start()
	facebookPostEmbeddingJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting Facebook post text embeddings backfill...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "error_message": nil,
		"reprocess_all": reqBody.ReprocessAll,
	})
	facebookPostEmbeddingJob.Broadcast("status", map[string]any{"status_line": "Starting Facebook post text embeddings backfill..."})
	go runFacebookPostEmbeddingBackfill(h.pool, h.embeddingSvc, facebookPostEmbeddingJob, uid, reqBody.ReprocessAll)
	writeJSON(w, map[string]any{"message": "Facebook post embedding backfill started", "status": "started", "reprocess_all": reqBody.ReprocessAll})
}

func (h *ImageHandler) FacebookPostEmbeddingsBackfillStream(w http.ResponseWriter, r *http.Request) {
	facebookPostEmbeddingJob.ServeSSE(w, r)
}

func (h *ImageHandler) FacebookPostEmbeddingsBackfillCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	writeJSON(w, facebookPostEmbeddingJob.Cancel())
}

func (h *ImageHandler) FacebookPostEmbeddingsBackfillStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, facebookPostEmbeddingJob.Status())
}

func runFacebookPostEmbeddingBackfill(pool *sql.DB, embeddingSvc *service.EmbeddingService, job *importer.ImportJob, uid int64, reprocessAll bool) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()
	if pool == nil || embeddingSvc == nil || !embeddingSvc.IsAvailable() {
		job.UpdateState(map[string]any{"status": "failed", "status_line": "Embedding service not available", "error_message": "embedding service unavailable"})
		job.Broadcast("failed", job.GetState())
		return
	}

	q := `
		SELECT fp.id, COALESCE(fp.post_text, '')
		FROM facebook_posts fp
		WHERE COALESCE(fp.user_id, 0) = ?1
		  AND TRIM(COALESCE(fp.post_text, '')) != ''`
	if !reprocessAll {
		q += ` AND NOT EXISTS (SELECT 1 FROM facebook_post_text_embeddings e WHERE e.rowid = fp.id)`
	}
	q += ` ORDER BY fp.id ASC`
	rows, err := pool.QueryContext(ctx, q, uid)
	if err != nil {
		job.UpdateState(map[string]any{"status": "failed", "status_line": fmt.Sprintf("Failed to list Facebook posts: %v", err), "error_message": err.Error()})
		job.Broadcast("failed", job.GetState())
		return
	}
	defer rows.Close()

	type row struct {
		id   int64
		text string
	}
	var list []row
	for rows.Next() {
		var rr row
		if scanErr := rows.Scan(&rr.id, &rr.text); scanErr != nil {
			job.UpdateState(map[string]any{"status": "failed", "status_line": fmt.Sprintf("Scan error: %v", scanErr), "error_message": scanErr.Error()})
			job.Broadcast("failed", job.GetState())
			return
		}
		list = append(list, rr)
	}
	if err := rows.Err(); err != nil {
		job.UpdateState(map[string]any{"status": "failed", "status_line": fmt.Sprintf("Iterate error: %v", err), "error_message": err.Error()})
		job.Broadcast("failed", job.GetState())
		return
	}

	total := len(list)
	job.UpdateState(map[string]any{"total": total, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "status_line": fmt.Sprintf("Facebook post embeddings: %d row(s) queued", total)})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, errorsCount := 0, 0, 0
	for i, rr := range list {
		if job.IsCancelled() {
			_ = recordImportControlLastRun(ctx, pool, uid, "facebook_post_text_embeddings", "cancelled", "cancelled")
			job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Facebook post embeddings backfill cancelled.", "processed": i, "embedded": embedded, "skipped": skipped, "errors": errorsCount})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		norm := normalizeFreeTextForEmbedding(rr.text)
		if norm == "" {
			skipped++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (skipped empty text)", i+1, total)})
			job.Broadcast("progress", job.GetState())
			continue
		}
		vec, err := embeddingSvc.EmbedText(ctx, norm)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (embedding failed id %d)", i+1, total, rr.id), "error_message": err.Error()})
			job.Broadcast("progress", job.GetState())
			continue
		}
		vecBlob, err := sqlite_vec.SerializeFloat32(vec)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (serialize failed id %d)", i+1, total, rr.id), "error_message": err.Error()})
			job.Broadcast("progress", job.GetState())
			continue
		}
		if err := sqlutil.Vec0Upsert(ctx, pool, "facebook_post_text_embeddings", rr.id, vecBlob, shortTextEmbeddingSignature(norm)); err != nil {
			errorsCount++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (upsert failed id %d)", i+1, total, rr.id), "error_message": err.Error()})
			job.Broadcast("progress", job.GetState())
			continue
		}
		embedded++
		job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (embedded post id %d)", i+1, total, rr.id)})
		job.Broadcast("progress", job.GetState())
	}

	_ = recordImportControlLastRun(ctx, pool, uid, "facebook_post_text_embeddings", "completed", "")
	job.UpdateState(map[string]any{"status": "completed", "status_line": fmt.Sprintf("Facebook post embedding backfill complete: %d embedded, %d skipped, %d errors", embedded, skipped, errorsCount), "processed": total, "embedded": embedded, "skipped": skipped, "errors": errorsCount})
	job.Broadcast("completed", job.GetState())
}

func (h *ImageHandler) SimilarFacebookPostsByText(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var req struct {
		Text string `json:"text"`
		N    int    `json:"n"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	norm := normalizeFreeTextForEmbedding(req.Text)
	if norm == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.N <= 0 {
		req.N = 25
	}
	if req.N > 50 {
		req.N = 50
	}
	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	vec, err := h.embeddingSvc.EmbedText(ctx, norm)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("embedding failed: %v", err))
		return
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("serialize embedding: %v", err))
		return
	}
	rows, err := h.pool.QueryContext(ctx, `
		SELECT fp.id, CAST(fp.timestamp AS TEXT), fp.title, fp.post_text, fp.external_url, fp.post_type, emb.distance
		FROM facebook_post_text_embeddings emb
		JOIN facebook_posts fp ON fp.id = emb.rowid
		WHERE emb.embedding MATCH ? AND emb.k = ?
		  AND COALESCE(fp.user_id, 0) = ?
		ORDER BY emb.distance ASC
		LIMIT ?`, vecBlob, vecKCandidate(req.N), uid, req.N)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}
	defer rows.Close()
	results := make([]map[string]any, 0, req.N)
	for rows.Next() {
		var id int64
		var ts sql.NullString
		var title, postText, externalURL, postType *string
		var distance any
		if err := rows.Scan(&id, &ts, &title, &postText, &externalURL, &postType, &distance); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan: %v", err))
			return
		}
		var tsOut any
		if ts.Valid {
			tsOut = ts.String
		}
		results = append(results, map[string]any{
			"id":           id,
			"timestamp":    tsOut,
			"title":        title,
			"post_text":    postText,
			"external_url": externalURL,
			"post_type":    postType,
			"distance":     jsonSafeVecDistance(distance),
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate: %v", err))
		return
	}
	writeJSON(w, map[string]any{"results": results, "count": len(results), "query_normalized": norm})
}

func (h *ImageHandler) FacebookAlbumEmbeddingsBackfillStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if err := facebookAlbumEmbeddingJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var reqBody struct {
		ReprocessAll bool `json:"reprocess_all"`
	}
	_ = json.NewDecoder(r.Body).Decode(&reqBody)
	uid := appctx.UserIDFromCtx(r.Context())
	facebookAlbumEmbeddingJob.Start()
	facebookAlbumEmbeddingJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting Facebook album description embeddings backfill...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "error_message": nil,
		"reprocess_all": reqBody.ReprocessAll,
	})
	facebookAlbumEmbeddingJob.Broadcast("status", map[string]any{"status_line": "Starting Facebook album description embeddings backfill..."})
	go runFacebookAlbumEmbeddingBackfill(h.pool, h.embeddingSvc, facebookAlbumEmbeddingJob, uid, reqBody.ReprocessAll)
	writeJSON(w, map[string]any{"message": "Facebook album embedding backfill started", "status": "started", "reprocess_all": reqBody.ReprocessAll})
}

func (h *ImageHandler) FacebookAlbumEmbeddingsBackfillStream(w http.ResponseWriter, r *http.Request) {
	facebookAlbumEmbeddingJob.ServeSSE(w, r)
}

func (h *ImageHandler) FacebookAlbumEmbeddingsBackfillCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	writeJSON(w, facebookAlbumEmbeddingJob.Cancel())
}

func (h *ImageHandler) FacebookAlbumEmbeddingsBackfillStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, facebookAlbumEmbeddingJob.Status())
}

func runFacebookAlbumEmbeddingBackfill(pool *sql.DB, embeddingSvc *service.EmbeddingService, job *importer.ImportJob, uid int64, reprocessAll bool) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()
	if pool == nil || embeddingSvc == nil || !embeddingSvc.IsAvailable() {
		job.UpdateState(map[string]any{"status": "failed", "status_line": "Embedding service not available", "error_message": "embedding service unavailable"})
		job.Broadcast("failed", job.GetState())
		return
	}
	q := `
		SELECT fa.id, COALESCE(fa.description, '')
		FROM facebook_albums fa
		WHERE COALESCE(fa.user_id, 0) = ?1
		  AND TRIM(COALESCE(fa.description, '')) != ''`
	if !reprocessAll {
		q += ` AND NOT EXISTS (SELECT 1 FROM facebook_album_description_embeddings e WHERE e.rowid = fa.id)`
	}
	q += ` ORDER BY fa.id ASC`
	rows, err := pool.QueryContext(ctx, q, uid)
	if err != nil {
		job.UpdateState(map[string]any{"status": "failed", "status_line": fmt.Sprintf("Failed to list Facebook albums: %v", err), "error_message": err.Error()})
		job.Broadcast("failed", job.GetState())
		return
	}
	defer rows.Close()
	type row struct {
		id   int64
		text string
	}
	var list []row
	for rows.Next() {
		var rr row
		if scanErr := rows.Scan(&rr.id, &rr.text); scanErr != nil {
			job.UpdateState(map[string]any{"status": "failed", "status_line": fmt.Sprintf("Scan error: %v", scanErr), "error_message": scanErr.Error()})
			job.Broadcast("failed", job.GetState())
			return
		}
		list = append(list, rr)
	}
	if err := rows.Err(); err != nil {
		job.UpdateState(map[string]any{"status": "failed", "status_line": fmt.Sprintf("Iterate error: %v", err), "error_message": err.Error()})
		job.Broadcast("failed", job.GetState())
		return
	}
	total := len(list)
	job.UpdateState(map[string]any{"total": total, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "status_line": fmt.Sprintf("Facebook album embeddings: %d row(s) queued", total)})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, errorsCount := 0, 0, 0
	for i, rr := range list {
		if job.IsCancelled() {
			_ = recordImportControlLastRun(ctx, pool, uid, "facebook_album_description_embeddings", "cancelled", "cancelled")
			job.UpdateState(map[string]any{"status": "cancelled", "status_line": "Facebook album embeddings backfill cancelled.", "processed": i, "embedded": embedded, "skipped": skipped, "errors": errorsCount})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		norm := normalizeFreeTextForEmbedding(rr.text)
		if norm == "" {
			skipped++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (skipped empty description)", i+1, total)})
			job.Broadcast("progress", job.GetState())
			continue
		}
		vec, err := embeddingSvc.EmbedText(ctx, norm)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (embedding failed id %d)", i+1, total, rr.id), "error_message": err.Error()})
			job.Broadcast("progress", job.GetState())
			continue
		}
		vecBlob, err := sqlite_vec.SerializeFloat32(vec)
		if err != nil {
			errorsCount++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (serialize failed id %d)", i+1, total, rr.id), "error_message": err.Error()})
			job.Broadcast("progress", job.GetState())
			continue
		}
		if err := sqlutil.Vec0Upsert(ctx, pool, "facebook_album_description_embeddings", rr.id, vecBlob, shortTextEmbeddingSignature(norm)); err != nil {
			errorsCount++
			job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (upsert failed id %d)", i+1, total, rr.id), "error_message": err.Error()})
			job.Broadcast("progress", job.GetState())
			continue
		}
		embedded++
		job.UpdateState(map[string]any{"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount, "status_line": fmt.Sprintf("Processed %d/%d (embedded album id %d)", i+1, total, rr.id)})
		job.Broadcast("progress", job.GetState())
	}

	_ = recordImportControlLastRun(ctx, pool, uid, "facebook_album_description_embeddings", "completed", "")
	job.UpdateState(map[string]any{"status": "completed", "status_line": fmt.Sprintf("Facebook album embedding backfill complete: %d embedded, %d skipped, %d errors", embedded, skipped, errorsCount), "processed": total, "embedded": embedded, "skipped": skipped, "errors": errorsCount})
	job.Broadcast("completed", job.GetState())
}

func (h *ImageHandler) SimilarFacebookAlbumsByDescription(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var req struct {
		Text string `json:"text"`
		N    int    `json:"n"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	norm := normalizeFreeTextForEmbedding(req.Text)
	if norm == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.N <= 0 {
		req.N = 25
	}
	if req.N > 50 {
		req.N = 50
	}
	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	vec, err := h.embeddingSvc.EmbedText(ctx, norm)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("embedding failed: %v", err))
		return
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("serialize embedding: %v", err))
		return
	}
	rows, err := h.pool.QueryContext(ctx, `
		SELECT fa.id, fa.name, fa.description, fa.cover_photo_uri, emb.distance
		FROM facebook_album_description_embeddings emb
		JOIN facebook_albums fa ON fa.id = emb.rowid
		WHERE emb.embedding MATCH ? AND emb.k = ?
		  AND COALESCE(fa.user_id, 0) = ?
		ORDER BY emb.distance ASC
		LIMIT ?`, vecBlob, vecKCandidate(req.N), uid, req.N)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}
	defer rows.Close()
	results := make([]map[string]any, 0, req.N)
	for rows.Next() {
		var id int64
		var name string
		var description, coverPhotoURI *string
		var distance any
		if err := rows.Scan(&id, &name, &description, &coverPhotoURI, &distance); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan: %v", err))
			return
		}
		results = append(results, map[string]any{
			"id":              id,
			"name":            name,
			"description":     description,
			"cover_photo_uri": coverPhotoURI,
			"distance":        jsonSafeVecDistance(distance),
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate: %v", err))
		return
	}
	writeJSON(w, map[string]any{"results": results, "count": len(results), "query_normalized": norm})
}

func (h *ImageHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
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
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
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
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseImageID(w, r, "image_id")
	if !ok {
		return
	}
	var payload struct {
		Description *string `json:"description"`
		Tags        any     `json:"tags"`
		Rating      any     `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	description := payload.Description
	tags, err := normalizePutImageTags(payload.Tags)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rating, err := normalizePutImageRating(payload.Rating)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
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
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
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

type ollamaImageMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaImageChatRequest struct {
	Model    string               `json:"model"`
	Messages []ollamaImageMessage `json:"messages"`
	Stream   bool                 `json:"stream"`
	Options  map[string]any       `json:"options,omitempty"`
}

type ollamaImageChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func classifyImageKeywordsWithOllama(ctx context.Context, encodedImageB64 string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("LOCALAI_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	reqBody := ollamaImageChatRequest{
		Model:  "gemma4:latest",
		Stream: false,
		Messages: []ollamaImageMessage{
			{
				Role: "user",
				Content: "Analyze this image and return only a JSON array of short keyword strings. " +
					"Capture content, atmosphere, vibe, and location. " +
					"Use 8 to 16 concise keywords. Do not include any text outside the JSON array.",
				Images: []string{encodedImageB64},
			},
		},
		Options: map[string]any{"temperature": 0.2},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal ollama image request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build ollama image request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama API status %d", resp.StatusCode)
	}
	var body ollamaImageChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	keywords, err := parseKeywordJSONArray(body.Message.Content)
	if err != nil {
		return "", fmt.Errorf("parse keyword array: %w", err)
	}
	return strings.Join(keywords, ", "), nil
}

func parseKeywordJSONArray(raw string) ([]string, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(arr))
	seen := make(map[string]struct{}, len(arr))
	for _, kw := range arr {
		k := strings.TrimSpace(kw)
		if k == "" {
			continue
		}
		key := strings.ToLower(k)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty keyword array")
	}
	return out, nil
}

func countNonEmptyCSV(csv string) int {
	parts := strings.Split(csv, ",")
	n := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			n++
		}
	}
	return n
}

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

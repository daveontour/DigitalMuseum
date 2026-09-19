package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/daveontour/aimuseum/internal/service/facerecognizer"
	"github.com/go-chi/chi/v5"
)

// faceEmbeddingModelName is recorded on every media_item_faces row so a
// future recognizer swap never gets silently compared against embeddings
// produced by a different model (see internal/database/migrate.go's
// media_item_faces.embedding_model column comment).
const faceEmbeddingModelName = "arcface-w600k_r50"

var faceDetectionJob = importer.NewImportJob("Face detection", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"total": 0, "processed": 0, "faces_found": 0, "errors": 0,
})

var faceClusteringJob = importer.NewImportJob("Face clustering", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"processed": 0, "joined_named": 0, "joined_other": 0, "new_clusters": 0, "errors": 0,
})

// runFaceDetection scans queued photos for faces via the bundled
// facerecognizer subprocess, storing one media_item_faces row and one
// face_embeddings vec0 row per detected face. cmd/facerecognizer and its
// bundled ONNX models are not part of this pass — until bin/FaceRecognizer/
// is staged, Start below returns facerecognizer.ErrNotAvailable and the job
// reports that clearly instead of running.
func runFaceDetection(
	faceRepo *repository.FaceRepo,
	embedHelper *service.FaceEmbeddingHelper,
	imageSvc *service.ImageService,
	job *importer.ImportJob,
	recognizerExePath string,
	uid int64,
	mediaItemIDs []int64,
) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	client := facerecognizer.NewClient(recognizerExePath)
	if err := client.Start(ctx); err != nil {
		job.UpdateState(map[string]any{
			"status":        "error",
			"status_line":   "Face recognition isn't set up yet — no bundled facerecognizer binary found.",
			"error_message": err.Error(),
		})
		job.Broadcast("error", job.GetState())
		return
	}
	defer func() { _ = client.Close() }()

	total := len(mediaItemIDs)
	processed, facesFound, errorsCount := 0, 0, 0

	for _, mediaItemID := range mediaItemIDs {
		if job.IsCancelled() {
			break
		}

		content, err := imageSvc.GetImageContent(ctx, mediaItemID, "metadata", false)
		if err != nil || content == nil || len(content.Data) == 0 {
			errorsCount++
			processed++
			continue
		}

		detected, err := client.Detect(content.Data)
		if err != nil {
			errorsCount++
			processed++
			continue
		}

		for _, df := range detected {
			confidence := df.Confidence
			var landmarksPtr *string
			if len(df.Landmarks) > 0 {
				if b, err := json.Marshal(df.Landmarks); err == nil {
					s := string(b)
					landmarksPtr = &s
				}
			}

			faceID, err := faceRepo.InsertFace(ctx, &model.Face{
				MediaItemID:         mediaItemID,
				BBoxX:               df.BBox.X,
				BBoxY:               df.BBox.Y,
				BBoxW:               df.BBox.W,
				BBoxH:               df.BBox.H,
				DetectionConfidence: &confidence,
				Landmarks:           landmarksPtr,
				EmbeddingModel:      faceEmbeddingModelName,
			})
			if err != nil {
				errorsCount++
				continue
			}
			if err := embedHelper.Sync(ctx, faceID, mediaItemID, df.Embedding); err != nil {
				errorsCount++
				continue
			}
			facesFound++
		}

		if err := faceRepo.MarkMediaItemFacesProcessed(ctx, mediaItemID); err != nil {
			errorsCount++
		}

		processed++
		job.UpdateState(map[string]any{
			"total": total, "processed": processed, "faces_found": facesFound, "errors": errorsCount,
			"status_line": fmt.Sprintf("Detecting faces: %d/%d photo(s) scanned, %d face(s) found", processed, total, facesFound),
		})
		job.Broadcast("progress", job.GetState())
	}

	status := "completed"
	if job.IsCancelled() {
		status = "cancelled"
	}
	job.UpdateState(map[string]any{
		"status": status,
		"status_line": fmt.Sprintf("Face detection finished: %d photo(s) scanned, %d face(s) found, %d error(s)",
			processed, facesFound, errorsCount),
	})
	job.Broadcast(status, job.GetState())
}

// runFaceClustering groups newly detected faces with already-named people
// and with each other, via FaceService.ClusterUnassignedFaces. Unlike face
// detection, this has no external dependency — it only needs the
// face_embeddings vec0 table, so it can run (and be exercised in tests)
// before cmd/facerecognizer exists.
func runFaceClustering(faceSvc *service.FaceService, job *importer.ImportJob, uid int64) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	stats, err := faceSvc.ClusterUnassignedFaces(ctx)
	if err != nil {
		job.UpdateState(map[string]any{"status": "error", "status_line": err.Error(), "error_message": err.Error()})
		job.Broadcast("error", job.GetState())
		return
	}

	job.UpdateState(map[string]any{
		"status":       "completed",
		"processed":    stats.Processed,
		"joined_named": stats.JoinedNamed,
		"joined_other": stats.JoinedOther,
		"new_clusters": stats.NewClusters,
		"errors":       stats.Errors,
		"status_line": fmt.Sprintf("Grouped %d face(s): %d matched a named person, %d joined other groups, %d new group(s)",
			stats.Processed, stats.JoinedNamed, stats.JoinedOther, stats.NewClusters),
	})
	job.Broadcast("completed", job.GetState())
}

// ── REST API: cluster review/naming, lightbox overlay ──────────────────────
//
// No "start detection/clustering job" routes are defined here — those already
// exist generically for every registered background job via
// BackgroundJobsHandler (Configuration → Background Jobs), so JobFaceDetection
// and JobFaceClustering are runnable from the UI without any face-specific
// route.

// faceCropOutputSize is the longest side, in pixels, of a face crop image
// served by ClusterThumbnail/FaceCrop — enough detail for a review grid
// without generating large responses.
const faceCropOutputSize = 240

// FaceHandler exposes the face-recognition cluster review/naming API and the
// per-photo face list used for the gallery lightbox overlay.
type FaceHandler struct {
	faceRepo    *repository.FaceRepo
	faceSvc     *service.FaceService
	imageSvc    *service.ImageService
	contactRepo *repository.ContactRepo
	embedHelper *service.FaceEmbeddingHelper
}

// NewFaceHandler creates a FaceHandler.
func NewFaceHandler(
	faceRepo *repository.FaceRepo,
	faceSvc *service.FaceService,
	imageSvc *service.ImageService,
	contactRepo *repository.ContactRepo,
	embedHelper *service.FaceEmbeddingHelper,
) *FaceHandler {
	return &FaceHandler{
		faceRepo:    faceRepo,
		faceSvc:     faceSvc,
		imageSvc:    imageSvc,
		contactRepo: contactRepo,
		embedHelper: embedHelper,
	}
}

// RegisterRoutes mounts the face-recognition review/naming routes.
func (h *FaceHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/faces/clusters", h.ListClusters)
	r.Get("/api/faces/clusters/{cluster_id}", h.GetCluster)
	r.Patch("/api/faces/clusters/{cluster_id}", h.PatchCluster)
	r.Get("/api/faces/clusters/{cluster_id}/thumbnail", h.ClusterThumbnail)
	r.Patch("/api/faces/{face_id}", h.PatchFace)
	r.Get("/api/faces/{face_id}/crop", h.FaceCrop)
	r.Get("/api/media-items/{media_item_id}/faces", h.ListFacesForMediaItem)
}

func parseFaceHandlerID(w http.ResponseWriter, r *http.Request, param, label string) (int64, bool) {
	raw := chi.URLParam(r, param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, label+" must be an integer")
		return 0, false
	}
	return id, true
}

func bboxJSON(f *model.Face) map[string]any {
	return map[string]any{"x": f.BBoxX, "y": f.BBoxY, "w": f.BBoxW, "h": f.BBoxH}
}

// defaultFaceClustersPageSize / maxFaceClustersPageSize bound how many
// clusters (and therefore how many on-demand ImageMagick thumbnail crops —
// see ClusterThumbnail) a single page of the "People in Photos" grid can
// trigger at once.
const (
	defaultFaceClustersPageSize = 48
	maxFaceClustersPageSize     = 200
)

// GET /api/faces/clusters?named=true|false&min_face_count=N&limit=N&offset=N
// Omit "named" to list every cluster. Ordered by face_count descending (see
// FaceRepo.ListClusters) so the most-photographed people surface first.
// min_face_count>1 restricts to clusters with more than one member face —
// the "Grouped (2+)" filter in the People in Photos UI, useful for skipping
// singleton clusters (often a single stray detection) when reviewing.
// limit defaults to defaultFaceClustersPageSize and is capped at
// maxFaceClustersPageSize; offset defaults to 0. Response includes "total"
// (the full count matching the filter, ignoring limit/offset) so the
// frontend can render page controls.
func (h *FaceHandler) ListClusters(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	namedOnly, unnamedOnly := false, false
	switch r.URL.Query().Get("named") {
	case "true":
		namedOnly = true
	case "false":
		unnamedOnly = true
	}

	minFaceCount := 0
	if v := r.URL.Query().Get("min_face_count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minFaceCount = n
		}
	}

	limit := defaultFaceClustersPageSize
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxFaceClustersPageSize {
		limit = maxFaceClustersPageSize
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	clusters, total, err := h.faceRepo.ListClusters(r.Context(), namedOnly, unnamedOnly, minFaceCount, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list clusters: "+err.Error())
		return
	}

	out := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, map[string]any{
			"id":                     c.ID,
			"contact_id":             c.ContactID,
			"contact_name":           c.ContactName,
			"face_count":             c.FaceCount,
			"thumbnail_url":          fmt.Sprintf("/api/faces/clusters/%d/thumbnail", c.ID),
			"representative_face_id": c.RepresentativeFaceID,
		})
	}
	writeJSON(w, map[string]any{
		"clusters": out,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// GET /api/faces/clusters/{cluster_id}
// Returns the cluster plus every member face (with a crop URL for the review
// grid) and, for an unnamed cluster, live top-contact suggestions computed
// from the representative face's nearest neighbors — not persisted anywhere,
// cheap enough to compute on every request at personal-archive scale.
func (h *FaceHandler) GetCluster(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	clusterID, ok := parseFaceHandlerID(w, r, "cluster_id", "cluster_id")
	if !ok {
		return
	}
	ctx := r.Context()

	cluster, err := h.faceRepo.GetCluster(ctx, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get cluster: "+err.Error())
		return
	}
	if cluster == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("cluster %d not found", clusterID))
		return
	}

	faces, err := h.faceRepo.ListFacesByCluster(ctx, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list cluster faces: "+err.Error())
		return
	}
	faceList := make([]map[string]any, 0, len(faces))
	for _, f := range faces {
		faceList = append(faceList, map[string]any{
			"id":            f.ID,
			"media_item_id": f.MediaItemID,
			"crop_url":      fmt.Sprintf("/api/faces/%d/crop", f.ID),
			"bbox":          bboxJSON(f),
			"confidence":    f.DetectionConfidence,
		})
	}

	var suggestions []map[string]any
	if cluster.ContactID == nil && cluster.RepresentativeFaceID != nil {
		suggestions = h.contactSuggestions(ctx, *cluster.RepresentativeFaceID)
	}

	writeJSON(w, map[string]any{
		"id":            cluster.ID,
		"contact_id":    cluster.ContactID,
		"contact_name":  cluster.ContactName,
		"face_count":    cluster.FaceCount,
		"thumbnail_url": fmt.Sprintf("/api/faces/clusters/%d/thumbnail", cluster.ID),
		"faces":         faceList,
		"suggestions":   suggestions,
	})
}

// contactSuggestions returns up to 5 distinct contacts, best-match first,
// found among the nearest-neighbor faces of representativeFaceID that are
// already linked to a contact — the same signal ClusterUnassignedFaces uses
// to auto-accept, surfaced here for a human to confirm instead.
func (h *FaceHandler) contactSuggestions(ctx context.Context, representativeFaceID int64) []map[string]any {
	if h.embedHelper == nil {
		return []map[string]any{}
	}
	matches, err := h.embedHelper.FindSimilarFaces(ctx, representativeFaceID, 10)
	if err != nil {
		return []map[string]any{}
	}

	type candidate struct {
		contactID int64
		distance  float64
	}
	bestByContact := map[int64]candidate{}
	for _, m := range matches {
		face, err := h.faceRepo.GetFace(ctx, m.FaceID)
		if err != nil || face == nil || face.ContactID == nil {
			continue
		}
		cid := *face.ContactID
		if existing, ok := bestByContact[cid]; !ok || m.Distance < existing.distance {
			bestByContact[cid] = candidate{contactID: cid, distance: m.Distance}
		}
	}

	list := make([]candidate, 0, len(bestByContact))
	for _, c := range bestByContact {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].distance < list[j].distance })
	if len(list) > 5 {
		list = list[:5]
	}

	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		name := ""
		if contact, err := h.contactRepo.GetContact(ctx, c.contactID); err == nil && contact != nil {
			name = contact.Name
		}
		out = append(out, map[string]any{
			"contact_id":   c.contactID,
			"contact_name": name,
			"distance":     c.distance,
		})
	}
	return out
}

// PATCH /api/faces/clusters/{cluster_id}
// Body: {"contact_id": 5} to link, {"contact_id": null} to unlink. Propagates
// to every member face — see FaceRepo.SetClusterContact.
func (h *FaceHandler) PatchCluster(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	clusterID, ok := parseFaceHandlerID(w, r, "cluster_id", "cluster_id")
	if !ok {
		return
	}
	var body struct {
		ContactID *int64 `json:"contact_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := r.Context()

	if body.ContactID != nil {
		exists, err := h.contactRepo.ContactExistsForUser(ctx, *body.ContactID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "check contact: "+err.Error())
			return
		}
		if !exists {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("contact %d not found", *body.ContactID))
			return
		}
	}

	if err := h.faceSvc.LinkClusterToContact(ctx, clusterID, body.ContactID); err != nil {
		writeError(w, http.StatusInternalServerError, "link cluster to contact: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// PATCH /api/faces/{face_id}
// Body: {"ignored": true} to permanently exclude this face (a false
// detection, or someone the user doesn't want tracked at all) — it is never
// re-homed by a later clustering run, unlike a plain detach. Otherwise,
// {"face_cluster_id": 7} to move to an existing cluster, or
// {"face_cluster_id": null} to detach ("not this person" — the next
// clustering job run re-homes it).
func (h *FaceHandler) PatchFace(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	faceID, ok := parseFaceHandlerID(w, r, "face_id", "face_id")
	if !ok {
		return
	}
	var body struct {
		FaceClusterID *int64 `json:"face_cluster_id"`
		Ignored       *bool  `json:"ignored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := r.Context()

	var err error
	switch {
	case body.Ignored != nil && *body.Ignored:
		err = h.faceSvc.IgnoreFace(ctx, faceID)
	case body.FaceClusterID == nil:
		err = h.faceSvc.DetachFace(ctx, faceID)
	default:
		err = h.faceSvc.MoveFaceToCluster(ctx, faceID, *body.FaceClusterID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update face: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// GET /api/faces/clusters/{cluster_id}/thumbnail — the cluster's
// representative face, cropped from its source photo.
func (h *FaceHandler) ClusterThumbnail(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	clusterID, ok := parseFaceHandlerID(w, r, "cluster_id", "cluster_id")
	if !ok {
		return
	}
	ctx := r.Context()

	cluster, err := h.faceRepo.GetCluster(ctx, clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get cluster: "+err.Error())
		return
	}
	if cluster == nil || cluster.RepresentativeFaceID == nil {
		writeError(w, http.StatusNotFound, "cluster has no representative face")
		return
	}
	h.serveFaceCrop(w, r, *cluster.RepresentativeFaceID)
}

// GET /api/faces/{face_id}/crop — one specific detected face, cropped from
// its source photo.
func (h *FaceHandler) FaceCrop(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	faceID, ok := parseFaceHandlerID(w, r, "face_id", "face_id")
	if !ok {
		return
	}
	h.serveFaceCrop(w, r, faceID)
}

func (h *FaceHandler) serveFaceCrop(w http.ResponseWriter, r *http.Request, faceID int64) {
	ctx := r.Context()
	face, err := h.faceRepo.GetFace(ctx, faceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get face: "+err.Error())
		return
	}
	if face == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("face %d not found", faceID))
		return
	}

	content, err := h.imageSvc.GetImageContent(ctx, face.MediaItemID, "metadata", false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load source photo: "+err.Error())
		return
	}
	if content == nil || len(content.Data) == 0 {
		writeError(w, http.StatusNotFound, "source photo not found")
		return
	}

	jpegBytes, err := service.CropFaceJPEG(content.Data, face.BBoxX, face.BBoxY, face.BBoxW, face.BBoxH, faceCropOutputSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "crop face: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(jpegBytes)
}

// GET /api/media-items/{media_item_id}/faces — every detected face for one
// photo, for the gallery lightbox's bounding-box overlay.
func (h *FaceHandler) ListFacesForMediaItem(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorContacts(w, r) {
		return
	}
	mediaItemID, ok := parseFaceHandlerID(w, r, "media_item_id", "media_item_id")
	if !ok {
		return
	}
	ctx := r.Context()

	faces, err := h.faceRepo.ListFacesByMediaItem(ctx, mediaItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list faces: "+err.Error())
		return
	}

	out := make([]map[string]any, 0, len(faces))
	for _, f := range faces {
		var contactName *string
		if f.ContactID != nil {
			if contact, err := h.contactRepo.GetContact(ctx, *f.ContactID); err == nil && contact != nil {
				contactName = &contact.Name
			}
		}
		out = append(out, map[string]any{
			"id":              f.ID,
			"bbox":            bboxJSON(f),
			"contact_id":      f.ContactID,
			"contact_name":    contactName,
			"face_cluster_id": f.FaceClusterID,
		})
	}
	writeJSON(w, out)
}

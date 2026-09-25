package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// TestFaceHandler_ListClusters_SuggestedFilter drives the "Possible Matches"
// filter end to end against a real on-disk SQLite file with real sqlite-vec
// face_embeddings rows. Seeds one named cluster, one unnamed cluster whose
// representative embedding is nearly identical to the named face's, and one
// whose embedding is orthogonal (far). Confirms: nothing is suggested until
// FaceService.RefreshClusterSuggestions runs; afterwards suggested=true
// returns only the near cluster with the named contact as its guess; plain
// named=false still returns both unnamed clusters; GetCluster's detail panel
// leads with the same stored guess; and naming the suggested cluster drops it
// from the filter immediately (the query requires contact_id IS NULL).
func TestFaceHandler_ListClusters_SuggestedFilter(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "archive.sqlite")

	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("open real db: %v", err)
	}
	if db == nil {
		t.Fatal("database.New returned nil db")
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.MigrateSQLite(ctx, db.Std); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	uid := int64(2)
	if _, err := db.Std.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, 'a@b.c', 'x', 'Test', 1, 0)`,
		uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)

	faceRepo := repository.NewFaceRepo(db.Std)
	faceEmbedHelper := service.NewFaceEmbeddingHelper(db.Std)
	faceSvc := service.NewFaceService(faceRepo, faceEmbedHelper)
	imageRepo := repository.NewImageRepo(db.Std)
	imageSvc := service.NewImageService(imageRepo, nil)
	contactRepo := repository.NewContactRepo(db.Std)

	h := NewFaceHandler(faceRepo, faceSvc, imageSvc, contactRepo, faceEmbedHelper, "")
	router := chi.NewRouter()
	h.RegisterRoutes(router)

	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req = req.WithContext(context.WithValue(req.Context(), appctx.ContextKeyUserID, uid))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	var contactID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Named Person', ?1) RETURNING id`, uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	var blobID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	newMediaItem := func() int64 {
		var id int64
		if err := db.Std.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, uid).Scan(&id); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		return id
	}
	confidence := 0.9
	newFace := func() (faceID, mediaItemID int64) {
		mediaItemID = newMediaItem()
		var err error
		faceID, err = faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
			BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace: %v", err)
		}
		return faceID, mediaItemID
	}

	// A 512-d unit vector with all its energy on axis 0.
	vecNamed := make([]float32, 512)
	vecNamed[0] = 1
	// Nearly identical to vecNamed (tiny perturbation on a different axis) —
	// should land as the nearest neighbor.
	vecClose := make([]float32, 512)
	vecClose[0] = 1
	vecClose[1] = 0.01
	// Orthogonal to vecNamed — far away in L2 distance.
	vecFar := make([]float32, 512)
	vecFar[2] = 1

	namedFaceID, _ := newFace()
	namedClusterID, err := faceRepo.CreateCluster(ctx, namedFaceID)
	if err != nil {
		t.Fatalf("CreateCluster (named): %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, namedFaceID, namedClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace (named): %v", err)
	}
	if err := faceRepo.SetClusterContact(ctx, namedClusterID, &contactID); err != nil {
		t.Fatalf("SetClusterContact: %v", err)
	}
	if err := faceEmbedHelper.Sync(ctx, namedFaceID, 0, vecNamed); err != nil {
		t.Fatalf("sync named embedding: %v", err)
	}

	closeFaceID, _ := newFace()
	closeClusterID, err := faceRepo.CreateCluster(ctx, closeFaceID)
	if err != nil {
		t.Fatalf("CreateCluster (close): %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, closeFaceID, closeClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace (close): %v", err)
	}
	if err := faceEmbedHelper.Sync(ctx, closeFaceID, 0, vecClose); err != nil {
		t.Fatalf("sync close embedding: %v", err)
	}

	farFaceID, _ := newFace()
	farClusterID, err := faceRepo.CreateCluster(ctx, farFaceID)
	if err != nil {
		t.Fatalf("CreateCluster (far): %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, farFaceID, farClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace (far): %v", err)
	}
	if err := faceEmbedHelper.Sync(ctx, farFaceID, 0, vecFar); err != nil {
		t.Fatalf("sync far embedding: %v", err)
	}

	// Before the "Find possible matches" job has run, nothing is suggested.
	rec := do(http.MethodGet, "/api/faces/clusters?suggested=true")
	if rec.Code != http.StatusOK || !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("ListClusters(suggested=true) before refresh: got %d: %s", rec.Code, rec.Body.String())
	}
	var before struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &before)
	if before.Total != 0 {
		t.Fatalf("want 0 suggested clusters before the refresh job runs, got %d", before.Total)
	}

	n, err := faceSvc.RefreshClusterSuggestions(ctx, nil, nil)
	if err != nil {
		t.Fatalf("RefreshClusterSuggestions: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 cluster with a stored suggestion, got %d", n)
	}

	type listResp struct {
		Clusters []struct {
			ID          int64  `json:"id"`
			ContactName string `json:"contact_name"`
			Suggestions []struct {
				ContactID   int64   `json:"contact_id"`
				ContactName string  `json:"contact_name"`
				Distance    float64 `json:"distance"`
			} `json:"suggestions"`
		} `json:"clusters"`
		Total int `json:"total"`
	}

	// suggested=true: only the close unnamed cluster should qualify, with
	// the named contact as its top guess.
	rec = do(http.MethodGet, "/api/faces/clusters?suggested=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(suggested=true): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var suggested listResp
	if err := json.Unmarshal(rec.Body.Bytes(), &suggested); err != nil {
		t.Fatalf("decode suggested response: %v", err)
	}
	if suggested.Total != 1 || len(suggested.Clusters) != 1 {
		t.Fatalf("want exactly 1 suggested cluster, got total=%d clusters=%+v", suggested.Total, suggested.Clusters)
	}
	got := suggested.Clusters[0]
	if got.ID != closeClusterID {
		t.Errorf("want the close cluster (%d) to be the suggested one, got %d", closeClusterID, got.ID)
	}
	if len(got.Suggestions) == 0 || got.Suggestions[0].ContactID != contactID || got.Suggestions[0].ContactName != "Named Person" {
		t.Errorf("want top suggestion = contact %d (Named Person), got %+v", contactID, got.Suggestions)
	}

	// The far cluster must never appear under suggested=true.
	for _, c := range suggested.Clusters {
		if c.ID == farClusterID {
			t.Errorf("far cluster %d should not have a suggestion", farClusterID)
		}
	}

	// Plain named=false (no suggested filter) must still return both unnamed
	// clusters — this filter narrows Unnamed, it doesn't replace it.
	rec = do(http.MethodGet, "/api/faces/clusters?named=false")
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(named=false): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var unnamed listResp
	if err := json.Unmarshal(rec.Body.Bytes(), &unnamed); err != nil {
		t.Fatalf("decode unnamed response: %v", err)
	}
	if unnamed.Total != 2 {
		t.Errorf("want 2 unnamed clusters without the suggested filter, got %d", unnamed.Total)
	}

	// The cluster detail panel must lead with the same stored guess.
	rec = do(http.MethodGet, fmt.Sprintf("/api/faces/clusters/%d", closeClusterID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GetCluster: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Suggestions []struct {
			ContactID int64 `json:"contact_id"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode GetCluster: %v", err)
	}
	if len(detail.Suggestions) == 0 || detail.Suggestions[0].ContactID != contactID {
		t.Errorf("want GetCluster's first suggestion = contact %d, got %+v", contactID, detail.Suggestions)
	}

	// Naming a suggested cluster removes it from Possible Matches immediately,
	// without waiting for the next refresh.
	if err := faceRepo.SetClusterContact(ctx, closeClusterID, &contactID); err != nil {
		t.Fatalf("SetClusterContact(close): %v", err)
	}
	rec = do(http.MethodGet, "/api/faces/clusters?suggested=true")
	var after listResp
	_ = json.Unmarshal(rec.Body.Bytes(), &after)
	if after.Total != 0 {
		t.Errorf("want 0 suggested clusters once the only one is named, got %d", after.Total)
	}
}

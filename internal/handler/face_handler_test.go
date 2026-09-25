package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// faceHandlerTestFixture bundles the router under test with direct DB access
// for seeding rows the handler itself doesn't create (photos, contacts,
// faces, clusters) — built against a real on-disk SQLite file via the real
// migration, matching the rigor used elsewhere in this codebase's face
// recognition tests, not a simplified stand-in schema.
type faceHandlerTestFixture struct {
	router chi.Router
	db     *sql.DB
	uid    int64
}

func newFaceHandlerTestFixture(t *testing.T) *faceHandlerTestFixture {
	t.Helper()
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

	faceRepo := repository.NewFaceRepo(db.Std)
	faceEmbedHelper := service.NewFaceEmbeddingHelper(db.Std)
	faceSvc := service.NewFaceService(faceRepo, faceEmbedHelper)
	imageRepo := repository.NewImageRepo(db.Std)
	imageSvc := service.NewImageService(imageRepo, nil)
	contactRepo := repository.NewContactRepo(db.Std)

	h := NewFaceHandler(faceRepo, faceSvc, imageSvc, contactRepo, faceEmbedHelper, "")
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	return &faceHandlerTestFixture{router: r, db: db.Std, uid: uid}
}

// withUID wraps req so it carries uid the way AuthMiddleware would in production.
func (f *faceHandlerTestFixture) withUID(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), appctx.ContextKeyUserID, f.uid))
}

func (f *faceHandlerTestFixture) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req = f.withUID(req)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// TestFaceHandler_ClusterReviewFlow drives the full cluster-review/naming
// REST API end to end against a real database: list clusters, inspect a
// cluster's member faces, link it to a contact (and confirm propagation to
// the member face), detach a face, and confirm the per-photo face list used
// by the gallery lightbox reflects all of it.
func TestFaceHandler_ClusterReviewFlow(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	var mediaItemID int64
	if err := f.db.QueryRowContext(ctx,
		`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
		blobID, f.uid,
	).Scan(&mediaItemID); err != nil {
		t.Fatalf("seed media_item: %v", err)
	}
	var contactID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Alice', ?1) RETURNING id`, f.uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9
	faceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: mediaItemID,
		BBoxX:       0.1, BBoxY: 0.2, BBoxW: 0.3, BBoxH: 0.4,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace: %v", err)
	}
	clusterID, err := faceRepo.CreateCluster(ctx, faceID)
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
	}

	// ── GET /api/faces/clusters ──────────────────────────────────────────
	rec := f.do(t, http.MethodGet, "/api/faces/clusters", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Clusters []struct {
			ID          int64   `json:"id"`
			ContactID   *int64  `json:"contact_id"`
			ContactName *string `json:"contact_name"`
			FaceCount   int     `json:"face_count"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode ListClusters response: %v", err)
	}
	if len(listResp.Clusters) != 1 || listResp.Clusters[0].ID != clusterID || listResp.Clusters[0].FaceCount != 1 {
		t.Fatalf("unexpected cluster list: %+v", listResp.Clusters)
	}
	if listResp.Clusters[0].ContactID != nil {
		t.Fatalf("want unlinked cluster, got contact_id=%v", listResp.Clusters[0].ContactID)
	}

	// ── GET /api/faces/clusters?named=true should now be empty ──────────
	rec = f.do(t, http.MethodGet, "/api/faces/clusters?named=true", nil)
	var namedResp struct {
		Clusters []any `json:"clusters"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &namedResp)
	if len(namedResp.Clusters) != 0 {
		t.Fatalf("want 0 named clusters before linking, got %d", len(namedResp.Clusters))
	}

	// ── GET /api/faces/clusters/{id} ─────────────────────────────────────
	rec = f.do(t, http.MethodGet, "/api/faces/clusters/"+itoa(clusterID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetCluster: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var detailResp struct {
		ID    int64 `json:"id"`
		Faces []struct {
			ID          int64  `json:"id"`
			MediaItemID int64  `json:"media_item_id"`
			CropURL     string `json:"crop_url"`
		} `json:"faces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("decode GetCluster response: %v", err)
	}
	if len(detailResp.Faces) != 1 || detailResp.Faces[0].ID != faceID || detailResp.Faces[0].MediaItemID != mediaItemID {
		t.Fatalf("unexpected cluster detail faces: %+v", detailResp.Faces)
	}
	if detailResp.Faces[0].CropURL == "" {
		t.Error("want a non-empty crop_url")
	}

	// ── PATCH /api/faces/clusters/{id} — link to contact ─────────────────
	rec = f.do(t, http.MethodPatch, "/api/faces/clusters/"+itoa(clusterID), map[string]any{"contact_id": contactID})
	if rec.Code != http.StatusOK {
		t.Fatalf("PatchCluster (link): want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Propagation check: the member face's own contact_id should now be set too.
	got, err := faceRepo.GetFace(ctx, faceID)
	if err != nil {
		t.Fatalf("GetFace after link: %v", err)
	}
	if got.ContactID == nil || *got.ContactID != contactID {
		t.Fatalf("want face.contact_id=%d after linking cluster, got %v", contactID, got.ContactID)
	}

	// Reject linking to a contact that doesn't belong to this user.
	rec = f.do(t, http.MethodPatch, "/api/faces/clusters/"+itoa(clusterID), map[string]any{"contact_id": 999999})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PatchCluster (bad contact): want 400, got %d: %s", rec.Code, rec.Body.String())
	}

	// ── GET /api/media-items/{id}/faces — lightbox overlay data ──────────
	rec = f.do(t, http.MethodGet, "/api/media-items/"+itoa(mediaItemID)+"/faces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListFacesForMediaItem: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var faceList []struct {
		ID          int64   `json:"id"`
		ContactName *string `json:"contact_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &faceList); err != nil {
		t.Fatalf("decode ListFacesForMediaItem response: %v", err)
	}
	if len(faceList) != 1 || faceList[0].ContactName == nil || *faceList[0].ContactName != "Alice" {
		t.Fatalf("unexpected lightbox face list: %+v", faceList)
	}

	// ── PATCH /api/faces/{id} — detach ("not this person") ───────────────
	rec = f.do(t, http.MethodPatch, "/api/faces/"+itoa(faceID), map[string]any{"face_cluster_id": nil})
	if rec.Code != http.StatusOK {
		t.Fatalf("PatchFace (detach): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got, err = faceRepo.GetFace(ctx, faceID)
	if err != nil {
		t.Fatalf("GetFace after detach: %v", err)
	}
	if got.FaceClusterID != nil || got.ContactID != nil {
		t.Fatalf("want detached face to have no cluster/contact, got cluster=%v contact=%v", got.FaceClusterID, got.ContactID)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

// TestFaceHandler_IgnoreFace covers the "ignore this face" action (a
// permanent exclusion, distinct from detach): ignoring one face in a
// two-face cluster removes it from the cluster, decrements face_count, and
// keeps it out of both the unclustered queue (so a later clustering run
// never re-homes it) and the lightbox face list for its photo. Ignoring the
// cluster's last remaining face empties it out, and an empty cluster no
// longer appears in ListClusters.
func TestFaceHandler_IgnoreFace(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	newMediaItem := func() int64 {
		var id int64
		if err := f.db.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, f.uid,
		).Scan(&id); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		return id
	}

	faceRepo := repository.NewFaceRepo(f.db)
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

	face1, mediaItem1 := newFace()
	face2, _ := newFace()

	clusterID, err := faceRepo.CreateCluster(ctx, face1)
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, face1, clusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
	}
	if err := faceRepo.AssignFaceToCluster(ctx, face2, clusterID); err != nil {
		t.Fatalf("AssignFaceToCluster: %v", err)
	}

	cluster, err := faceRepo.GetCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("GetCluster (before ignore): %v", err)
	}
	if cluster.FaceCount != 2 {
		t.Fatalf("want face_count=2 before ignoring, got %d", cluster.FaceCount)
	}

	// ── PATCH /api/faces/{id} — ignore face1 ──────────────────────────────
	rec := f.do(t, http.MethodPatch, "/api/faces/"+itoa(face1), map[string]any{"ignored": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("PatchFace (ignore): want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got, err := faceRepo.GetFace(ctx, face1)
	if err != nil {
		t.Fatalf("GetFace after ignore: %v", err)
	}
	if !got.Ignored {
		t.Error("want face1.Ignored=true")
	}
	if got.FaceClusterID != nil || got.ContactID != nil {
		t.Errorf("want ignored face to have no cluster/contact, got cluster=%v contact=%v", got.FaceClusterID, got.ContactID)
	}

	cluster, err = faceRepo.GetCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("GetCluster (after ignoring 1 of 2): %v", err)
	}
	if cluster.FaceCount != 1 {
		t.Fatalf("want face_count=1 after ignoring one of two faces, got %d", cluster.FaceCount)
	}

	// An ignored face must never be picked up for (re-)clustering.
	unclustered, err := faceRepo.ListUnclusteredFaceIDs(ctx)
	if err != nil {
		t.Fatalf("ListUnclusteredFaceIDs: %v", err)
	}
	for _, id := range unclustered {
		if id == face1 {
			t.Error("ignored face1 must not appear in ListUnclusteredFaceIDs")
		}
	}

	// An ignored face must not appear in its photo's lightbox face list.
	rec = f.do(t, http.MethodGet, "/api/media-items/"+itoa(mediaItem1)+"/faces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListFacesForMediaItem: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var faceList []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &faceList); err != nil {
		t.Fatalf("decode ListFacesForMediaItem response: %v", err)
	}
	if len(faceList) != 0 {
		t.Errorf("want ignored face excluded from lightbox list, got %+v", faceList)
	}

	// ── Ignore the cluster's last remaining face ──────────────────────────
	rec = f.do(t, http.MethodPatch, "/api/faces/"+itoa(face2), map[string]any{"ignored": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("PatchFace (ignore face2): want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = f.do(t, http.MethodGet, "/api/faces/clusters", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Clusters []struct {
			ID int64 `json:"id"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode ListClusters response: %v", err)
	}
	for _, c := range listResp.Clusters {
		if c.ID == clusterID {
			t.Errorf("want emptied-out cluster %d excluded from ListClusters, got it in the list", clusterID)
		}
	}
}

// TestFaceHandler_IgnoreUnnamedFace_LeavesNamedFaceInSamePhotoUntouched
// reproduces the "Ignore unnamed people" button's exact frontend flow — GET
// the photo's face list, PATCH-ignore only the face(s) the client sees as
// unnamed (contact_id == nil), then re-GET — for a photo containing both a
// named (contact-linked) face and a genuinely unnamed one, to confirm
// ignoring the unnamed face never disturbs the named face's cluster/contact
// link or its presence in the lightbox list.
func TestFaceHandler_IgnoreUnnamedFace_LeavesNamedFaceInSamePhotoUntouched(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	var mediaItemID int64
	if err := f.db.QueryRowContext(ctx,
		`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
		blobID, f.uid,
	).Scan(&mediaItemID); err != nil {
		t.Fatalf("seed media_item: %v", err)
	}
	var contactID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Dave Burton', ?1) RETURNING id`, f.uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9
	namedFaceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: mediaItemID,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (named): %v", err)
	}
	clusterID, err := faceRepo.CreateCluster(ctx, namedFaceID)
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, namedFaceID, clusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
	}
	if err := faceRepo.SetClusterContact(ctx, clusterID, &contactID); err != nil {
		t.Fatalf("SetClusterContact: %v", err)
	}

	unnamedFaceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: mediaItemID,
		BBoxX:       0.6, BBoxY: 0.6, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (unnamed): %v", err)
	}

	// ── Step 1: GET the photo's face list, exactly as the Image Details
	// dialog's overlay does, and derive the "unnamed" set the same way the
	// frontend's currentUnnamedFaceIds does (contact_id == nil).
	rec := f.do(t, http.MethodGet, "/api/media-items/"+itoa(mediaItemID)+"/faces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListFacesForMediaItem: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var faceList []struct {
		ID            int64   `json:"id"`
		ContactID     *int64  `json:"contact_id"`
		ContactName   *string `json:"contact_name"`
		FaceClusterID *int64  `json:"face_cluster_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &faceList); err != nil {
		t.Fatalf("decode ListFacesForMediaItem response: %v", err)
	}
	if len(faceList) != 2 {
		t.Fatalf("want 2 faces in photo, got %d: %+v", len(faceList), faceList)
	}
	var unnamedIDs []int64
	for _, fc := range faceList {
		if fc.ContactID == nil {
			unnamedIDs = append(unnamedIDs, fc.ID)
		} else if fc.ID == namedFaceID && (fc.ContactName == nil || *fc.ContactName != "Dave Burton") {
			t.Fatalf("want named face to report contact_name=Dave Burton, got %+v", fc)
		}
	}
	if len(unnamedIDs) != 1 || unnamedIDs[0] != unnamedFaceID {
		t.Fatalf("want exactly [%d] classified unnamed, got %v", unnamedFaceID, unnamedIDs)
	}

	// ── Step 2: PATCH-ignore only the unnamed face(s), as the "Ignore
	// unnamed people" button does.
	for _, id := range unnamedIDs {
		rec = f.do(t, http.MethodPatch, "/api/faces/"+itoa(id), map[string]any{"ignored": true})
		if rec.Code != http.StatusOK {
			t.Fatalf("PatchFace (ignore %d): want 200, got %d: %s", id, rec.Code, rec.Body.String())
		}
	}

	// ── Step 3: re-GET the photo's face list — the named face must still be
	// present, still linked to its contact, and the ignored face must be
	// gone.
	rec = f.do(t, http.MethodGet, "/api/media-items/"+itoa(mediaItemID)+"/faces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListFacesForMediaItem (after ignore): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var faceListAfter []struct {
		ID          int64   `json:"id"`
		ContactID   *int64  `json:"contact_id"`
		ContactName *string `json:"contact_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &faceListAfter); err != nil {
		t.Fatalf("decode ListFacesForMediaItem (after ignore) response: %v", err)
	}
	if len(faceListAfter) != 1 {
		t.Fatalf("want exactly 1 face remaining (the named one), got %d: %+v", len(faceListAfter), faceListAfter)
	}
	got := faceListAfter[0]
	if got.ID != namedFaceID {
		t.Fatalf("want remaining face to be the named one (%d), got %d", namedFaceID, got.ID)
	}
	if got.ContactID == nil || *got.ContactID != contactID {
		t.Errorf("want named face still linked to contact %d, got %v", contactID, got.ContactID)
	}
	if got.ContactName == nil || *got.ContactName != "Dave Burton" {
		t.Errorf("want named face to still report contact_name=Dave Burton, got %v", got.ContactName)
	}

	cluster, err := faceRepo.GetCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("GetCluster (after ignore): %v", err)
	}
	if cluster.ContactID == nil || *cluster.ContactID != contactID {
		t.Errorf("want named face's cluster still linked to contact %d, got %v", contactID, cluster.ContactID)
	}
}

// TestFaceHandler_ListClusters_Pagination seeds five singleton clusters and
// drives GET /api/faces/clusters across three pages of two, confirming: each
// page returns exactly `limit` clusters (the last, partial one aside), pages
// don't repeat or skip clusters, ordering is stable across pages (by
// face_count desc, id asc — all these clusters tie on face_count=1, so this
// also confirms the id-asc tiebreaker keeps paging stable), "total" reflects
// the full filtered count regardless of limit/offset, and an out-of-range
// offset returns an empty page rather than erroring.
func TestFaceHandler_ListClusters_Pagination(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9

	const numClusters = 5
	clusterIDs := make([]int64, 0, numClusters)
	for i := 0; i < numClusters; i++ {
		var mediaItemID int64
		if err := f.db.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, f.uid,
		).Scan(&mediaItemID); err != nil {
			t.Fatalf("seed media_item %d: %v", i, err)
		}
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
			BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace %d: %v", i, err)
		}
		clusterID, err := faceRepo.CreateCluster(ctx, faceID)
		if err != nil {
			t.Fatalf("CreateCluster %d: %v", i, err)
		}
		if err := faceRepo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
			t.Fatalf("SetNewClusterRepresentativeFace %d: %v", i, err)
		}
		clusterIDs = append(clusterIDs, clusterID)
	}

	type pageResp struct {
		Clusters []struct {
			ID int64 `json:"id"`
		} `json:"clusters"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}

	fetchPage := func(limit, offset int) pageResp {
		rec := f.do(t, http.MethodGet,
			"/api/faces/clusters?limit="+itoa(int64(limit))+"&offset="+itoa(int64(offset)), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("ListClusters(limit=%d,offset=%d): want 200, got %d: %s", limit, offset, rec.Code, rec.Body.String())
		}
		var resp pageResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode ListClusters response: %v", err)
		}
		return resp
	}

	// Expected order is ascending cluster id, since every cluster here ties
	// on face_count=1 (the primary sort key) and id ASC is the tiebreaker.
	wantOrder := append([]int64(nil), clusterIDs...)

	var seen []int64
	for page := 0; page < 3; page++ { // pages of 2 over 5 clusters: 2, 2, 1
		resp := fetchPage(2, page*2)
		if resp.Total != numClusters {
			t.Errorf("page %d: want total=%d, got %d", page, numClusters, resp.Total)
		}
		if resp.Limit != 2 || resp.Offset != page*2 {
			t.Errorf("page %d: want limit=2 offset=%d echoed back, got limit=%d offset=%d", page, page*2, resp.Limit, resp.Offset)
		}
		wantLen := 2
		if page == 2 {
			wantLen = 1 // last, partial page
		}
		if len(resp.Clusters) != wantLen {
			t.Fatalf("page %d: want %d clusters, got %d", page, wantLen, len(resp.Clusters))
		}
		for _, c := range resp.Clusters {
			seen = append(seen, c.ID)
		}
	}

	if len(seen) != numClusters {
		t.Fatalf("want %d clusters seen across all pages, got %d: %v", numClusters, len(seen), seen)
	}
	for i, id := range seen {
		if id != wantOrder[i] {
			t.Errorf("cluster order mismatch at position %d: got %v, want %v", i, seen, wantOrder)
			break
		}
	}

	// An offset past the end returns an empty page, not an error, with the
	// correct (unchanged) total.
	resp := fetchPage(2, 100)
	if len(resp.Clusters) != 0 {
		t.Errorf("want empty page for out-of-range offset, got %d clusters", len(resp.Clusters))
	}
	if resp.Total != numClusters {
		t.Errorf("want total=%d even for an out-of-range offset, got %d", numClusters, resp.Total)
	}

	// A named-only filter combined with pagination should report a total of
	// 0 (none of these clusters are linked to a contact) and no server error.
	rec := f.do(t, http.MethodGet, "/api/faces/clusters?named=true&limit=2&offset=0", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(named=true): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var namedResp pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &namedResp); err != nil {
		t.Fatalf("decode named ListClusters response: %v", err)
	}
	if namedResp.Total != 0 || len(namedResp.Clusters) != 0 {
		t.Errorf("want 0 named clusters/total, got total=%d clusters=%d", namedResp.Total, len(namedResp.Clusters))
	}
}

// TestFaceHandler_ListClusters_MinFaceCountFilter seeds two singleton
// clusters (face_count=1) and one cluster with two member faces
// (face_count=2), then confirms min_face_count=2 returns only the grouped
// cluster while the unfiltered list still returns all three.
func TestFaceHandler_ListClusters_MinFaceCountFilter(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9

	newFace := func() int64 {
		var mediaItemID int64
		if err := f.db.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, f.uid,
		).Scan(&mediaItemID); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
			BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace: %v", err)
		}
		return faceID
	}

	// Two singleton clusters.
	for i := 0; i < 2; i++ {
		faceID := newFace()
		clusterID, err := faceRepo.CreateCluster(ctx, faceID)
		if err != nil {
			t.Fatalf("CreateCluster: %v", err)
		}
		if err := faceRepo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
			t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
		}
	}

	// One cluster with two member faces.
	groupedFace1 := newFace()
	groupedClusterID, err := faceRepo.CreateCluster(ctx, groupedFace1)
	if err != nil {
		t.Fatalf("CreateCluster (grouped): %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, groupedFace1, groupedClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace (grouped): %v", err)
	}
	groupedFace2 := newFace()
	if err := faceRepo.AssignFaceToCluster(ctx, groupedFace2, groupedClusterID); err != nil {
		t.Fatalf("AssignFaceToCluster (grouped): %v", err)
	}

	type pageResp struct {
		Clusters []struct {
			ID        int64 `json:"id"`
			FaceCount int   `json:"face_count"`
		} `json:"clusters"`
		Total int `json:"total"`
	}

	rec := f.do(t, http.MethodGet, "/api/faces/clusters?min_face_count=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(min_face_count=2): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var filtered pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered ListClusters response: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Clusters) != 1 {
		t.Fatalf("want exactly 1 grouped cluster, got total=%d clusters=%d", filtered.Total, len(filtered.Clusters))
	}
	if filtered.Clusters[0].ID != groupedClusterID {
		t.Errorf("want grouped cluster %d, got %d", groupedClusterID, filtered.Clusters[0].ID)
	}
	if filtered.Clusters[0].FaceCount != 2 {
		t.Errorf("want face_count=2, got %d", filtered.Clusters[0].FaceCount)
	}

	// Unfiltered (min_face_count omitted) should return all three clusters.
	rec = f.do(t, http.MethodGet, "/api/faces/clusters", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(unfiltered): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var unfiltered pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &unfiltered); err != nil {
		t.Fatalf("decode unfiltered ListClusters response: %v", err)
	}
	if unfiltered.Total != 3 {
		t.Errorf("want total=3 unfiltered, got %d", unfiltered.Total)
	}
}

// TestFaceHandler_ListClusters_MaxFaceCountFilter reuses the same two
// singletons + one 2-face cluster fixture as the min_face_count test above,
// confirming max_face_count=1 returns only the two singleton clusters, and
// that combining it with min_face_count=1 (an explicit "size == 1" range)
// gives the identical result.
func TestFaceHandler_ListClusters_MaxFaceCountFilter(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9

	newFace := func() int64 {
		var mediaItemID int64
		if err := f.db.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, f.uid,
		).Scan(&mediaItemID); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
			BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace: %v", err)
		}
		return faceID
	}

	for i := 0; i < 2; i++ {
		faceID := newFace()
		clusterID, err := faceRepo.CreateCluster(ctx, faceID)
		if err != nil {
			t.Fatalf("CreateCluster: %v", err)
		}
		if err := faceRepo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
			t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
		}
	}

	groupedFace1 := newFace()
	groupedClusterID, err := faceRepo.CreateCluster(ctx, groupedFace1)
	if err != nil {
		t.Fatalf("CreateCluster (grouped): %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, groupedFace1, groupedClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace (grouped): %v", err)
	}
	groupedFace2 := newFace()
	if err := faceRepo.AssignFaceToCluster(ctx, groupedFace2, groupedClusterID); err != nil {
		t.Fatalf("AssignFaceToCluster (grouped): %v", err)
	}

	type pageResp struct {
		Clusters []struct {
			ID        int64 `json:"id"`
			FaceCount int   `json:"face_count"`
		} `json:"clusters"`
		Total int `json:"total"`
	}

	fetchAndCheckSingletonsOnly := func(t *testing.T, query string) {
		t.Helper()
		rec := f.do(t, http.MethodGet, "/api/faces/clusters?"+query, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("ListClusters(%s): want 200, got %d: %s", query, rec.Code, rec.Body.String())
		}
		var resp pageResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode ListClusters(%s) response: %v", query, err)
		}
		if resp.Total != 2 || len(resp.Clusters) != 2 {
			t.Fatalf("ListClusters(%s): want exactly 2 singleton clusters, got total=%d clusters=%d", query, resp.Total, len(resp.Clusters))
		}
		for _, c := range resp.Clusters {
			if c.ID == groupedClusterID {
				t.Errorf("ListClusters(%s): grouped cluster %d should have been excluded", query, groupedClusterID)
			}
			if c.FaceCount != 1 {
				t.Errorf("ListClusters(%s): want face_count=1, got %d for cluster %d", query, c.FaceCount, c.ID)
			}
		}
	}

	fetchAndCheckSingletonsOnly(t, "max_face_count=1")
	fetchAndCheckSingletonsOnly(t, "min_face_count=1&max_face_count=1")

	// An impossible range (min > max) degrades to an empty result, not an error.
	rec := f.do(t, http.MethodGet, "/api/faces/clusters?min_face_count=5&max_face_count=1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(min=5,max=1): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var empty pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode ListClusters(min=5,max=1) response: %v", err)
	}
	if empty.Total != 0 || len(empty.Clusters) != 0 {
		t.Fatalf("want empty result for an impossible min>max range, got total=%d clusters=%d", empty.Total, len(empty.Clusters))
	}
}

// TestFaceHandler_ListClusters_ContactNameFilter seeds three named clusters
// (Blake Whitney, Blake Anderson, Dave Burton) and one unnamed cluster, then
// confirms contact_name=blake case-insensitively substring-matches only the
// two Blakes, contact_name combined with an unmatched name returns empty,
// and an unnamed cluster never matches any non-empty contact_name filter
// regardless of namedOnly.
func TestFaceHandler_ListClusters_ContactNameFilter(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9

	newFace := func() int64 {
		var mediaItemID int64
		if err := f.db.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, f.uid,
		).Scan(&mediaItemID); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
			BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace: %v", err)
		}
		return faceID
	}

	newNamedCluster := func(name string) int64 {
		var contactID int64
		if err := f.db.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES (?1, ?2) RETURNING id`, name, f.uid).Scan(&contactID); err != nil {
			t.Fatalf("seed contact %q: %v", name, err)
		}
		faceID := newFace()
		clusterID, err := faceRepo.CreateCluster(ctx, faceID)
		if err != nil {
			t.Fatalf("CreateCluster: %v", err)
		}
		if err := faceRepo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
			t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
		}
		if err := faceRepo.SetClusterContact(ctx, clusterID, &contactID); err != nil {
			t.Fatalf("SetClusterContact: %v", err)
		}
		return clusterID
	}

	blakeWhitneyID := newNamedCluster("Blake Whitney")
	blakeAndersonID := newNamedCluster("Blake Anderson")
	newNamedCluster("Dave Burton")

	// Unnamed cluster.
	unnamedFace := newFace()
	unnamedClusterID, err := faceRepo.CreateCluster(ctx, unnamedFace)
	if err != nil {
		t.Fatalf("CreateCluster (unnamed): %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, unnamedFace, unnamedClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace (unnamed): %v", err)
	}

	type pageResp struct {
		Clusters []struct {
			ID          int64   `json:"id"`
			ContactName *string `json:"contact_name"`
		} `json:"clusters"`
		Total int `json:"total"`
	}

	// Case-insensitive substring match against both Blakes.
	rec := f.do(t, http.MethodGet, "/api/faces/clusters?contact_name="+url.QueryEscape("blake"), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(contact_name=blake): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var blakeResp pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &blakeResp); err != nil {
		t.Fatalf("decode contact_name=blake response: %v", err)
	}
	gotIDs := map[int64]bool{}
	for _, c := range blakeResp.Clusters {
		gotIDs[c.ID] = true
	}
	if blakeResp.Total != 2 || !gotIDs[blakeWhitneyID] || !gotIDs[blakeAndersonID] {
		t.Fatalf("want exactly [%d,%d] for contact_name=blake, got total=%d ids=%v", blakeWhitneyID, blakeAndersonID, blakeResp.Total, gotIDs)
	}

	// A more specific match narrows to one.
	rec = f.do(t, http.MethodGet, "/api/faces/clusters?contact_name="+url.QueryEscape("Whitney"), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(contact_name=Whitney): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var whitneyResp pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &whitneyResp); err != nil {
		t.Fatalf("decode contact_name=Whitney response: %v", err)
	}
	if whitneyResp.Total != 1 || len(whitneyResp.Clusters) != 1 || whitneyResp.Clusters[0].ID != blakeWhitneyID {
		t.Fatalf("want exactly [%d] for contact_name=Whitney, got %+v", blakeWhitneyID, whitneyResp)
	}

	// A name matching nobody returns an empty result, not an error.
	rec = f.do(t, http.MethodGet, "/api/faces/clusters?contact_name="+url.QueryEscape("Nonexistent"), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListClusters(contact_name=Nonexistent): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var noneResp pageResp
	if err := json.Unmarshal(rec.Body.Bytes(), &noneResp); err != nil {
		t.Fatalf("decode contact_name=Nonexistent response: %v", err)
	}
	if noneResp.Total != 0 || len(noneResp.Clusters) != 0 {
		t.Fatalf("want empty result for an unmatched contact_name, got total=%d clusters=%d", noneResp.Total, len(noneResp.Clusters))
	}

	// The unnamed cluster is never swept up by a non-empty contact_name
	// filter, even without combining it with named=true.
	for _, c := range blakeResp.Clusters {
		if c.ID == unnamedClusterID {
			t.Error("unnamed cluster must never match a contact_name filter")
		}
	}
}

// TestFaceHandler_FaceCrop_ServesStoredCropDirectly confirms GET
// /api/faces/{id}/crop serves a pre-stored crop_data blob byte-for-byte
// without falling back to fetching the source photo / invoking ImageMagick
// — the fast path added so viewing People in Photos doesn't regenerate
// every crop on every request. Deliberately seeds a face with crop_data set
// directly (bypassing InsertFace/CropFaceJPEG) so this test exercises the
// HTTP-serving logic in isolation and needs no real image data or the
// bundled ImageMagick binary to pass.
func TestFaceHandler_FaceCrop_ServesStoredCropDirectly(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	var mediaItemID int64
	if err := f.db.QueryRowContext(ctx,
		`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
		blobID, f.uid,
	).Scan(&mediaItemID); err != nil {
		t.Fatalf("seed media_item: %v", err)
	}

	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9
	storedCrop := []byte("fake-jpeg-bytes-stored-crop")
	faceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: mediaItemID,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
		CropData:            storedCrop,
	})
	if err != nil {
		t.Fatalf("InsertFace: %v", err)
	}

	rec := f.do(t, http.MethodGet, "/api/faces/"+itoa(faceID)+"/crop", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("FaceCrop: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, storedCrop) {
		t.Fatalf("FaceCrop: want stored crop bytes %q, got %q", storedCrop, got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("FaceCrop: want Content-Type image/jpeg, got %q", ct)
	}
}

// TestFaceHandler_IdentifyFace drives POST /api/faces/{id}/identify end to
// end: naming an unclustered face (must create a singleton cluster), a
// missing/invalid contact_id (400), and a nonexistent contact (400) — the
// flow behind the Image Details dialog's "identify unnamed people" feature.
func TestFaceHandler_IdentifyFace(t *testing.T) {
	f := newFaceHandlerTestFixture(t)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, f.uid)

	var blobID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, f.uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	var mediaItemID int64
	if err := f.db.QueryRowContext(ctx,
		`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
		blobID, f.uid,
	).Scan(&mediaItemID); err != nil {
		t.Fatalf("seed media_item: %v", err)
	}
	var contactID int64
	if err := f.db.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Blake Whitney', ?1) RETURNING id`, f.uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	faceRepo := repository.NewFaceRepo(f.db)
	confidence := 0.9
	faceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: mediaItemID,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace: %v", err)
	}

	// Missing contact_id.
	rec := f.do(t, http.MethodPost, "/api/faces/"+itoa(faceID)+"/identify", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("IdentifyFace (missing contact_id): want 400, got %d: %s", rec.Code, rec.Body.String())
	}

	// Nonexistent contact.
	rec = f.do(t, http.MethodPost, "/api/faces/"+itoa(faceID)+"/identify", map[string]any{"contact_id": contactID + 9999})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("IdentifyFace (bad contact): want 400, got %d: %s", rec.Code, rec.Body.String())
	}

	// Success: names the previously-unclustered face.
	rec = f.do(t, http.MethodPost, "/api/faces/"+itoa(faceID)+"/identify", map[string]any{"contact_id": contactID})
	if rec.Code != http.StatusOK {
		t.Fatalf("IdentifyFace: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		OK        bool  `json:"ok"`
		ClusterID int64 `json:"cluster_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode IdentifyFace response: %v", err)
	}
	if !resp.OK || resp.ClusterID == 0 {
		t.Fatalf("want ok=true and a non-zero cluster_id, got %+v", resp)
	}

	// The face now carries the contact and is surfaced as a named cluster.
	updated, err := faceRepo.GetFace(ctx, faceID)
	if err != nil {
		t.Fatalf("GetFace: %v", err)
	}
	if updated.ContactID == nil || *updated.ContactID != contactID {
		t.Fatalf("want face linked to contact %d, got %v", contactID, updated.ContactID)
	}
	if updated.FaceClusterID == nil || *updated.FaceClusterID != resp.ClusterID {
		t.Fatalf("want face's cluster to be %d, got %v", resp.ClusterID, updated.FaceClusterID)
	}
}

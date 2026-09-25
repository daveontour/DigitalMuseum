package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// TestClusterUnassignedFaces_RealDB exercises the full clustering pipeline
// (FaceService + FaceEmbeddingHelper + FaceRepo + the real face_embeddings
// vec0 table) against an actual on-disk SQLite file created via the real
// database.New + database.MigrateSQLite path — the same rigor that caught
// the DBTime scan bug in FaceRepo, applied one layer up. No real
// facerecognizer subprocess is needed: embeddings are hand-constructed
// vectors simulating "two photos of the same person" (near-identical
// vectors) and "a different person" (an orthogonal vector).
func TestClusterUnassignedFaces_RealDB(t *testing.T) {
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
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)

	if _, err := db.Std.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, 'a@b.c', 'x', 'Test', 1, 0)`, uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var blobID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	newMediaItem := func() int64 {
		var id int64
		if err := db.Std.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, uid,
		).Scan(&id); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		return id
	}
	var contactID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Alice', ?1) RETURNING id`, uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	faceRepo := repository.NewFaceRepo(db.Std)
	embedHelper := NewFaceEmbeddingHelper(db.Std)
	faceSvc := NewFaceService(faceRepo, embedHelper)

	insertFaceWithEmbedding := func(embedding []float32) int64 {
		confidence := 0.95
		mediaItemID := newMediaItem()
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID:         mediaItemID,
			BBoxX:                0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace: %v", err)
		}
		if err := embedHelper.Sync(ctx, faceID, mediaItemID, embedding); err != nil {
			t.Fatalf("embedHelper.Sync: %v", err)
		}
		return faceID
	}

	// face_embeddings is declared float[512] (the real ArcFace-class output
	// dimension — see ensureFaceEmbeddingsVecTable), so test vectors must
	// match that width; only the first two components carry the "signal".
	vec512 := func(a, b float32) []float32 {
		v := make([]float32, 512)
		v[0], v[1] = a, b
		return v
	}

	// Two faces of "the same person" — near-identical embeddings.
	personAFace1 := insertFaceWithEmbedding(vec512(1, 0))
	personAFace2 := insertFaceWithEmbedding(vec512(0.99, 0.01))
	// A different person — an orthogonal embedding, far from both above.
	personBFace1 := insertFaceWithEmbedding(vec512(0, 1))

	stats, err := faceSvc.ClusterUnassignedFaces(ctx)
	if err != nil {
		t.Fatalf("ClusterUnassignedFaces: %v", err)
	}
	if stats.Errors != 0 {
		t.Fatalf("want 0 errors, got %d", stats.Errors)
	}
	if stats.Processed != 3 {
		t.Fatalf("want 3 faces processed, got %d", stats.Processed)
	}
	// Expect 2 clusters overall (A's two faces together, B alone), though the
	// exact split between "new" and "joined" depends on processing order —
	// what matters is the end state, checked below.
	if stats.NewClusters+stats.JoinedOther+stats.JoinedNamed != 3 {
		t.Fatalf("stats don't add up to 3 outcomes: %+v", stats)
	}

	faceA1, err := faceRepo.GetFace(ctx, personAFace1)
	if err != nil {
		t.Fatalf("GetFace personAFace1: %v", err)
	}
	faceA2, err := faceRepo.GetFace(ctx, personAFace2)
	if err != nil {
		t.Fatalf("GetFace personAFace2: %v", err)
	}
	faceB1, err := faceRepo.GetFace(ctx, personBFace1)
	if err != nil {
		t.Fatalf("GetFace personBFace1: %v", err)
	}

	if faceA1.FaceClusterID == nil || faceA2.FaceClusterID == nil || faceB1.FaceClusterID == nil {
		t.Fatalf("every face should have been assigned a cluster: a1=%v a2=%v b1=%v",
			faceA1.FaceClusterID, faceA2.FaceClusterID, faceB1.FaceClusterID)
	}
	if *faceA1.FaceClusterID != *faceA2.FaceClusterID {
		t.Errorf("want personA's two faces in the same cluster, got %d vs %d", *faceA1.FaceClusterID, *faceA2.FaceClusterID)
	}
	if *faceA1.FaceClusterID == *faceB1.FaceClusterID {
		t.Errorf("want personB's face in a different cluster from personA, both got cluster %d", *faceA1.FaceClusterID)
	}

	// Name personA's cluster, then add a third near-identical face and
	// re-cluster: it should auto-join the now-named cluster.
	if err := faceSvc.LinkClusterToContact(ctx, *faceA1.FaceClusterID, &contactID); err != nil {
		t.Fatalf("LinkClusterToContact: %v", err)
	}
	personAFace3 := insertFaceWithEmbedding(vec512(0.98, 0.02))

	stats2, err := faceSvc.ClusterUnassignedFaces(ctx)
	if err != nil {
		t.Fatalf("ClusterUnassignedFaces (second run): %v", err)
	}
	if stats2.Processed != 1 || stats2.JoinedNamed != 1 {
		t.Fatalf("want the new face to auto-join the named cluster, got %+v", stats2)
	}

	faceA3, err := faceRepo.GetFace(ctx, personAFace3)
	if err != nil {
		t.Fatalf("GetFace personAFace3: %v", err)
	}
	if faceA3.FaceClusterID == nil || *faceA3.FaceClusterID != *faceA1.FaceClusterID {
		t.Fatalf("want personA's third face in cluster %d, got %v", *faceA1.FaceClusterID, faceA3.FaceClusterID)
	}
	if faceA3.ContactID == nil || *faceA3.ContactID != contactID {
		t.Fatalf("want personA's third face to inherit the linked contact, got %v", faceA3.ContactID)
	}
}

// TestIdentifyFace_RealDB covers FaceService.IdentifyFace's two paths — a
// face that already belongs to a cluster (just links that cluster, same as
// LinkClusterToContact) and a face detected but not yet run through
// ClusterUnassignedFaces (no face_cluster_id yet), which must get a new
// singleton cluster created for it first. This is what backs the Image
// Details dialog's "identify unnamed people" flow.
func TestIdentifyFace_RealDB(t *testing.T) {
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
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)
	if _, err := db.Std.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, 'a@b.c', 'x', 'Test', 1, 0)`,
		uid,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var blobID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	newMediaItem := func() int64 {
		var id int64
		if err := db.Std.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
			blobID, uid,
		).Scan(&id); err != nil {
			t.Fatalf("seed media_item: %v", err)
		}
		return id
	}
	var aliceID, bobID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Alice', ?1) RETURNING id`, uid).Scan(&aliceID); err != nil {
		t.Fatalf("seed contact Alice: %v", err)
	}
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Bob', ?1) RETURNING id`, uid).Scan(&bobID); err != nil {
		t.Fatalf("seed contact Bob: %v", err)
	}

	faceRepo := repository.NewFaceRepo(db.Std)
	faceSvc := NewFaceService(faceRepo, nil)
	confidence := 0.9

	// Case 1: a face with NO cluster yet (fresh detection, clustering job
	// hasn't run). IdentifyFace must create a singleton cluster for it.
	unclusteredFaceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: newMediaItem(),
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (unclustered): %v", err)
	}
	beforeFace, err := faceRepo.GetFace(ctx, unclusteredFaceID)
	if err != nil {
		t.Fatalf("GetFace (before identify): %v", err)
	}
	if beforeFace.FaceClusterID != nil {
		t.Fatalf("want no cluster before IdentifyFace, got %v", beforeFace.FaceClusterID)
	}

	newClusterID, err := faceSvc.IdentifyFace(ctx, unclusteredFaceID, aliceID)
	if err != nil {
		t.Fatalf("IdentifyFace (unclustered): %v", err)
	}
	afterFace, err := faceRepo.GetFace(ctx, unclusteredFaceID)
	if err != nil {
		t.Fatalf("GetFace (after identify): %v", err)
	}
	if afterFace.FaceClusterID == nil || *afterFace.FaceClusterID != newClusterID {
		t.Fatalf("want face assigned to new cluster %d, got %v", newClusterID, afterFace.FaceClusterID)
	}
	if afterFace.ContactID == nil || *afterFace.ContactID != aliceID {
		t.Fatalf("want face linked to Alice (%d), got %v", aliceID, afterFace.ContactID)
	}
	cluster, err := faceRepo.GetCluster(ctx, newClusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if cluster == nil || cluster.ContactID == nil || *cluster.ContactID != aliceID {
		t.Fatalf("want new cluster linked to Alice, got %+v", cluster)
	}
	if cluster.RepresentativeFaceID == nil || *cluster.RepresentativeFaceID != unclusteredFaceID {
		t.Fatalf("want the identified face set as the new cluster's representative, got %v", cluster.RepresentativeFaceID)
	}

	// Case 2: a face that ALREADY belongs to a cluster (e.g. an unnamed
	// cluster with several faces already grouped by the clustering job).
	// IdentifyFace must link that existing cluster, not create a new one,
	// and the link must propagate to every member face.
	clusteredFace1ID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: newMediaItem(),
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (clustered 1): %v", err)
	}
	existingClusterID, err := faceRepo.CreateCluster(ctx, clusteredFace1ID)
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := faceRepo.SetNewClusterRepresentativeFace(ctx, clusteredFace1ID, existingClusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
	}
	clusteredFace2ID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: newMediaItem(),
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (clustered 2): %v", err)
	}
	if err := faceRepo.AssignFaceToCluster(ctx, clusteredFace2ID, existingClusterID); err != nil {
		t.Fatalf("AssignFaceToCluster: %v", err)
	}

	gotClusterID, err := faceSvc.IdentifyFace(ctx, clusteredFace1ID, bobID)
	if err != nil {
		t.Fatalf("IdentifyFace (clustered): %v", err)
	}
	if gotClusterID != existingClusterID {
		t.Fatalf("want the existing cluster %d reused, got %d", existingClusterID, gotClusterID)
	}
	member2, err := faceRepo.GetFace(ctx, clusteredFace2ID)
	if err != nil {
		t.Fatalf("GetFace (member 2): %v", err)
	}
	if member2.ContactID == nil || *member2.ContactID != bobID {
		t.Fatalf("want the contact link to propagate to every member face, got %v", member2.ContactID)
	}
}

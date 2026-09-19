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

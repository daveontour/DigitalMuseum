package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/model"
)

// TestFaceRepo_RealDB exercises FaceRepo against an actual on-disk SQLite
// file created via the real database.New + database.MigrateSQLite path (not
// a hand-rolled test schema), to catch any mismatch between the Go SQL and
// what the real migration actually produces — e.g. the sender_id-type scan
// bug found earlier in this codebase's message-dedup logic came from exactly
// this kind of gap between a hand-rolled test schema and the real one.
func TestFaceRepo_RealDB(t *testing.T) {
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

	// Confirm the new schema pieces actually exist.
	var n int
	for _, table := range []string{"face_clusters", "media_item_faces", "face_embeddings"} {
		if err := db.Std.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table') AND name = ?`, table,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master %s: %v", table, err)
		}
		if n == 0 {
			t.Fatalf("expected table %s to exist after migration", table)
		}
	}
	if err := db.Std.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('media_items') WHERE name = 'faces_processed'`,
	).Scan(&n); err != nil {
		t.Fatalf("pragma_table_info media_items: %v", err)
	}
	if n == 0 {
		t.Fatal("expected media_items.faces_processed column to exist after migration")
	}

	// Seed a minimal media_blobs/media_items/contacts row to attach faces to,
	// scoped to a fake authenticated user so uid-filtered queries exercise
	// the same path production traffic does.
	uid := int64(2)
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)

	if _, err := db.Std.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, 'a@b.c', 'x', 'Test', 1, 0)`, uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var blobID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	var mediaItemID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
		blobID, uid,
	).Scan(&mediaItemID); err != nil {
		t.Fatalf("seed media_item: %v", err)
	}
	var contactID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Alice', ?1) RETURNING id`, uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	repo := NewFaceRepo(db.Std)

	// ListImageIDsForFaceDetection should surface the seeded photo (faces_processed defaults to false).
	ids, err := repo.ListImageIDsForFaceDetection(ctx)
	if err != nil {
		t.Fatalf("ListImageIDsForFaceDetection: %v", err)
	}
	if len(ids) != 1 || ids[0] != mediaItemID {
		t.Fatalf("want [%d], got %v", mediaItemID, ids)
	}

	// InsertFace + GetFace round-trip.
	confidence := 0.97
	landmarks := `[[0.1,0.1],[0.2,0.1],[0.15,0.15],[0.12,0.2],[0.18,0.2]]`
	faceID, err := repo.InsertFace(ctx, &model.Face{
		MediaItemID:         mediaItemID,
		BBoxX:                0.1, BBoxY: 0.2, BBoxW: 0.3, BBoxH: 0.4,
		DetectionConfidence: &confidence,
		Landmarks:           &landmarks,
		EmbeddingModel:      "arcface-w600k_r50",
	})
	if err != nil {
		t.Fatalf("InsertFace: %v", err)
	}

	got, err := repo.GetFace(ctx, faceID)
	if err != nil {
		t.Fatalf("GetFace: %v", err)
	}
	if got == nil {
		t.Fatal("GetFace returned nil for a face that was just inserted")
	}
	if got.MediaItemID != mediaItemID || got.BBoxX != 0.1 || got.EmbeddingModel != "arcface-w600k_r50" {
		t.Fatalf("unexpected face row: %+v", got)
	}
	if got.FaceClusterID != nil || got.ContactID != nil {
		t.Fatalf("newly inserted face should start unclustered, got cluster=%v contact=%v", got.FaceClusterID, got.ContactID)
	}

	// MarkMediaItemFacesProcessed should remove it from the detection queue.
	if err := repo.MarkMediaItemFacesProcessed(ctx, mediaItemID); err != nil {
		t.Fatalf("MarkMediaItemFacesProcessed: %v", err)
	}
	ids, err = repo.ListImageIDsForFaceDetection(ctx)
	if err != nil {
		t.Fatalf("ListImageIDsForFaceDetection (after mark): %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("want empty queue after marking processed, got %v", ids)
	}

	// ListUnclusteredFaceIDs should surface our face.
	unclustered, err := repo.ListUnclusteredFaceIDs(ctx)
	if err != nil {
		t.Fatalf("ListUnclusteredFaceIDs: %v", err)
	}
	if len(unclustered) != 1 || unclustered[0] != faceID {
		t.Fatalf("want [%d], got %v", faceID, unclustered)
	}

	// CreateCluster + SetNewClusterRepresentativeFace, then link to a contact
	// and confirm propagation to the member face (SetClusterContact).
	clusterID, err := repo.CreateCluster(ctx, faceID)
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := repo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
		t.Fatalf("SetNewClusterRepresentativeFace: %v", err)
	}

	unclustered, err = repo.ListUnclusteredFaceIDs(ctx)
	if err != nil {
		t.Fatalf("ListUnclusteredFaceIDs (after cluster): %v", err)
	}
	if len(unclustered) != 0 {
		t.Fatalf("want no unclustered faces left, got %v", unclustered)
	}

	if err := repo.SetClusterContact(ctx, clusterID, &contactID); err != nil {
		t.Fatalf("SetClusterContact: %v", err)
	}

	got, err = repo.GetFace(ctx, faceID)
	if err != nil {
		t.Fatalf("GetFace (after contact link): %v", err)
	}
	if got.FaceClusterID == nil || *got.FaceClusterID != clusterID {
		t.Fatalf("want face_cluster_id=%d, got %v", clusterID, got.FaceClusterID)
	}
	if got.ContactID == nil || *got.ContactID != contactID {
		t.Fatalf("want contact_id=%d propagated from cluster, got %v", contactID, got.ContactID)
	}

	cluster, err := repo.GetCluster(ctx, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if cluster == nil {
		t.Fatal("GetCluster returned nil")
	}
	if cluster.ContactName == nil || *cluster.ContactName != "Alice" {
		t.Fatalf("want joined contact name Alice, got %v", cluster.ContactName)
	}
	if cluster.FaceCount != 1 {
		t.Fatalf("want face_count=1, got %d", cluster.FaceCount)
	}

	// DetachFace should clear both assignments.
	if err := repo.DetachFace(ctx, faceID); err != nil {
		t.Fatalf("DetachFace: %v", err)
	}
	got, err = repo.GetFace(ctx, faceID)
	if err != nil {
		t.Fatalf("GetFace (after detach): %v", err)
	}
	if got.FaceClusterID != nil || got.ContactID != nil {
		t.Fatalf("want detached face to have no cluster/contact, got cluster=%v contact=%v", got.FaceClusterID, got.ContactID)
	}
}

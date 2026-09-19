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

// TestImageRepo_Search_PersonFilter exercises ImageSearchParams.Person against
// a real on-disk SQLite file via the actual database.New + database.MigrateSQLite
// path (not a hand-rolled test schema — see TestFaceRepo_RealDB's comment on why
// this matters in this codebase), seeding two photos, only one of which has a
// media_item_faces row linked to a named contact, and confirming the person
// filter isolates exactly that photo — matching on both a full and partial name,
// and excluding an ignored face row from matching at all.
func TestImageRepo_Search_PersonFilter(t *testing.T) {
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

	newMediaItem := func(title string) int64 {
		var id int64
		if err := db.Std.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, title, user_id) VALUES (?1, 'image/jpeg', ?2, ?3) RETURNING id`,
			blobID, title, uid,
		).Scan(&id); err != nil {
			t.Fatalf("seed media_item %q: %v", title, err)
		}
		return id
	}

	photoOfAlice := newMediaItem("Photo of Alice")
	photoOfNobody := newMediaItem("Photo of nobody in particular")

	var aliceID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Alice Smith', ?1) RETURNING id`, uid).Scan(&aliceID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	faceRepo := NewFaceRepo(db.Std)
	confidence := 0.95
	faceID, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: photoOfAlice,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
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
	if err := faceRepo.SetClusterContact(ctx, clusterID, &aliceID); err != nil {
		t.Fatalf("SetClusterContact: %v", err)
	}

	// An ignored face linked to a different contact on the "nobody" photo must
	// never match — IgnoreFace clears its cluster/contact_id, so this also
	// doubles as a check that the filter doesn't get confused by a stray face
	// row with no contact.
	ignoredFace, err := faceRepo.InsertFace(ctx, &model.Face{
		MediaItemID: photoOfNobody,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (ignored): %v", err)
	}
	if err := faceRepo.IgnoreFace(ctx, ignoredFace); err != nil {
		t.Fatalf("IgnoreFace: %v", err)
	}

	imageRepo := NewImageRepo(db.Std)

	for _, name := range []string{"Alice Smith", "Alice", "alice"} {
		p := name
		results, err := imageRepo.Search(ctx, model.ImageSearchParams{Person: &p})
		if err != nil {
			t.Fatalf("Search(person=%q): %v", name, err)
		}
		if len(results) != 1 {
			t.Fatalf("Search(person=%q): want 1 result, got %d", name, len(results))
		}
		if results[0].ID != photoOfAlice {
			t.Errorf("Search(person=%q): want media_item %d, got %d", name, photoOfAlice, results[0].ID)
		}
	}

	noMatch := "Bob"
	results, err := imageRepo.Search(ctx, model.ImageSearchParams{Person: &noMatch})
	if err != nil {
		t.Fatalf("Search(person=Bob): %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("Search(person=Bob): want 0 results, got %d", len(results))
	}
}

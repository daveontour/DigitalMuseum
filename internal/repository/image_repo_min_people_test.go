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

// TestImageRepo_Search_MinPeopleCountFilter exercises
// ImageSearchParams.MinPeopleCount against a real on-disk SQLite file via
// the actual database.New + database.MigrateSQLite path (see
// TestFaceRepo_RealDB's comment on why this matters in this codebase).
// Seeds three photos with 0, 1, and 2 detected (non-ignored) faces plus one
// ignored face thrown in on the 2-face photo, and confirms min_people_count
// correctly isolates photos with at least that many people, counting faces
// regardless of whether they've been named, and never counting an ignored
// face.
func TestImageRepo_Search_MinPeopleCountFilter(t *testing.T) {
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

	faceRepo := NewFaceRepo(db.Std)
	confidence := 0.9
	insertFace := func(mediaItemID int64) int64 {
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

	photoZeroPeople := newMediaItem("No people")
	photoOnePerson := newMediaItem("One person")
	insertFace(photoOnePerson)

	photoTwoPeople := newMediaItem("Two people")
	insertFace(photoTwoPeople)
	insertFace(photoTwoPeople)
	// An ignored face on the same photo must never count toward the total.
	ignored := insertFace(photoTwoPeople)
	if err := faceRepo.IgnoreFace(ctx, ignored); err != nil {
		t.Fatalf("IgnoreFace: %v", err)
	}

	imageRepo := NewImageRepo(db.Std)

	one := 1
	results, err := imageRepo.Search(ctx, model.ImageSearchParams{MinPeopleCount: &one})
	if err != nil {
		t.Fatalf("Search(min_people_count=1): %v", err)
	}
	gotIDs := map[int64]bool{}
	for _, r := range results {
		gotIDs[r.ID] = true
	}
	if len(results) != 2 || !gotIDs[photoOnePerson] || !gotIDs[photoTwoPeople] {
		t.Fatalf("Search(min_people_count=1): want [%d,%d], got %v", photoOnePerson, photoTwoPeople, gotIDs)
	}
	if gotIDs[photoZeroPeople] {
		t.Errorf("Search(min_people_count=1): photo with no faces should not match")
	}

	two := 2
	results, err = imageRepo.Search(ctx, model.ImageSearchParams{MinPeopleCount: &two})
	if err != nil {
		t.Fatalf("Search(min_people_count=2): %v", err)
	}
	if len(results) != 1 || results[0].ID != photoTwoPeople {
		t.Fatalf("Search(min_people_count=2): want [%d], got %v", photoTwoPeople, results)
	}

	three := 3
	results, err = imageRepo.Search(ctx, model.ImageSearchParams{MinPeopleCount: &three})
	if err != nil {
		t.Fatalf("Search(min_people_count=3): %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("Search(min_people_count=3): want 0 results (ignored face must not count), got %v", results)
	}
}

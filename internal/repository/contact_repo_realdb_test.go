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

// TestContactRepo_CreateContact exercises NameExists + CreateContact against
// a real on-disk SQLite file via the actual database.New + database.MigrateSQLite
// path (not a hand-rolled test schema — see TestFaceRepo_RealDB's comment on
// why this matters in this codebase), confirming a brand-new name can be
// created, a case-insensitive duplicate is detected via NameExists, and the
// created row round-trips through GetContact.
func TestContactRepo_CreateContact(t *testing.T) {
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

	repo := NewContactRepo(db.Std)

	exists, err := repo.NameExists(ctx, "New Person")
	if err != nil {
		t.Fatalf("NameExists (before create): %v", err)
	}
	if exists {
		t.Fatal("want NameExists=false before the contact is created")
	}

	created, err := repo.CreateContact(ctx, "New Person")
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if created.ID == 0 || created.Name != "New Person" {
		t.Fatalf("want a valid id and name %q, got %+v", "New Person", created)
	}

	// Case-insensitive duplicate detection.
	exists, err = repo.NameExists(ctx, "new person")
	if err != nil {
		t.Fatalf("NameExists (after create, case-insensitive): %v", err)
	}
	if !exists {
		t.Fatal("want NameExists=true (case-insensitive) after the contact is created")
	}

	// Round-trips through GetContact.
	got, err := repo.GetContact(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if got == nil || got.Name != "New Person" {
		t.Fatalf("GetContact: want %q, got %+v", "New Person", got)
	}
}

// TestContactRepo_ListShort_NumPhotos seeds one contact linked to two photos
// via face recognition (media_item_faces.contact_id), one contact with an
// ignored face only (must not count), and one contact with no faces at all,
// then confirms ListShort's numphotos column reports the right count for
// each — 2, 0, and 0 respectively.
func TestContactRepo_ListShort_NumPhotos(t *testing.T) {
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

	contactRepo := NewContactRepo(db.Std)
	faceRepo := NewFaceRepo(db.Std)
	confidence := 0.9

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

	var twoPhotoContactID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Has Photos', ?1) RETURNING id`, uid).Scan(&twoPhotoContactID); err != nil {
		t.Fatalf("seed contact (photos): %v", err)
	}
	for i := 0; i < 2; i++ {
		mediaItemID := newMediaItem()
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
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
		if err := faceRepo.SetClusterContact(ctx, clusterID, &twoPhotoContactID); err != nil {
			t.Fatalf("SetClusterContact: %v", err)
		}
	}

	var ignoredOnlyContactID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('Ignored Only', ?1) RETURNING id`, uid).Scan(&ignoredOnlyContactID); err != nil {
		t.Fatalf("seed contact (ignored): %v", err)
	}
	{
		mediaItemID := newMediaItem()
		faceID, err := faceRepo.InsertFace(ctx, &model.Face{
			MediaItemID: mediaItemID,
			BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
			DetectionConfidence: &confidence,
			EmbeddingModel:      "test-model",
		})
		if err != nil {
			t.Fatalf("InsertFace (ignored): %v", err)
		}
		clusterID, err := faceRepo.CreateCluster(ctx, faceID)
		if err != nil {
			t.Fatalf("CreateCluster (ignored): %v", err)
		}
		if err := faceRepo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
			t.Fatalf("SetNewClusterRepresentativeFace (ignored): %v", err)
		}
		if err := faceRepo.SetClusterContact(ctx, clusterID, &ignoredOnlyContactID); err != nil {
			t.Fatalf("SetClusterContact (ignored): %v", err)
		}
		if err := faceRepo.IgnoreFace(ctx, faceID); err != nil {
			t.Fatalf("IgnoreFace: %v", err)
		}
	}

	var noPhotosContactID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('No Photos', ?1) RETURNING id`, uid).Scan(&noPhotosContactID); err != nil {
		t.Fatalf("seed contact (none): %v", err)
	}

	contacts, total, err := contactRepo.ListShort(ctx, ContactListParams{})
	if err != nil {
		t.Fatalf("ListShort: %v", err)
	}
	if total != 3 {
		t.Fatalf("want total=3, got %d", total)
	}

	byID := make(map[int64]*model.Contact, len(contacts))
	for _, c := range contacts {
		byID[c.ID] = c
	}
	if c := byID[twoPhotoContactID]; c == nil || c.NumPhotos != 2 {
		t.Errorf("want NumPhotos=2 for 'Has Photos', got %+v", c)
	}
	if c := byID[ignoredOnlyContactID]; c == nil || c.NumPhotos != 0 {
		t.Errorf("want NumPhotos=0 for 'Ignored Only' (ignored face must not count), got %+v", c)
	}
	if c := byID[noPhotosContactID]; c == nil || c.NumPhotos != 0 {
		t.Errorf("want NumPhotos=0 for 'No Photos', got %+v", c)
	}
}

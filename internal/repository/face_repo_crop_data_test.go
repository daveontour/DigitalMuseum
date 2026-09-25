package repository

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/model"
)

// TestFaceRepo_CropData exercises the stored-crop path added to speed up
// FaceHandler.serveFaceCrop (crop_data on media_item_faces — see
// InsertFace/GetFaceCropData/SetFaceCropData/ListFacesMissingCropData),
// against a real on-disk SQLite file via the actual database.New +
// database.MigrateSQLite path (see TestFaceRepo_RealDB's comment on why
// this matters in this codebase).
func TestFaceRepo_CropData(t *testing.T) {
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

	var n int
	if err := db.Std.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('media_item_faces') WHERE name = 'crop_data'`,
	).Scan(&n); err != nil {
		t.Fatalf("pragma_table_info media_item_faces.crop_data: %v", err)
	}
	if n == 0 {
		t.Fatal("expected media_item_faces.crop_data column to exist after migration")
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

	repo := NewFaceRepo(db.Std)
	confidence := 0.9

	// A face inserted WITH crop data (the new detection-time path).
	withCropMediaItemID := newMediaItem()
	precomputedCrop := []byte("fake-jpeg-bytes-1")
	faceWithCropID, err := repo.InsertFace(ctx, &model.Face{
		MediaItemID: withCropMediaItemID,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
		CropData:            precomputedCrop,
	})
	if err != nil {
		t.Fatalf("InsertFace (with crop): %v", err)
	}

	got, err := repo.GetFaceCropData(ctx, faceWithCropID)
	if err != nil {
		t.Fatalf("GetFaceCropData (with crop): %v", err)
	}
	if !bytes.Equal(got, precomputedCrop) {
		t.Fatalf("GetFaceCropData: want %q, got %q", precomputedCrop, got)
	}

	// A face inserted WITHOUT crop data (a legacy face, or a failed on-the-fly crop).
	noCropMediaItemID := newMediaItem()
	faceNoCropID, err := repo.InsertFace(ctx, &model.Face{
		MediaItemID: noCropMediaItemID,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (no crop): %v", err)
	}

	got, err = repo.GetFaceCropData(ctx, faceNoCropID)
	if err != nil {
		t.Fatalf("GetFaceCropData (no crop): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetFaceCropData (no crop): want empty, got %q", got)
	}

	// An ignored face without crop data — must not show up in the backfill list.
	ignoredMediaItemID := newMediaItem()
	ignoredFaceID, err := repo.InsertFace(ctx, &model.Face{
		MediaItemID: ignoredMediaItemID,
		BBoxX:       0.1, BBoxY: 0.1, BBoxW: 0.2, BBoxH: 0.2,
		DetectionConfidence: &confidence,
		EmbeddingModel:      "test-model",
	})
	if err != nil {
		t.Fatalf("InsertFace (ignored): %v", err)
	}
	if err := repo.IgnoreFace(ctx, ignoredFaceID); err != nil {
		t.Fatalf("IgnoreFace: %v", err)
	}

	missing, err := repo.ListFacesMissingCropData(ctx)
	if err != nil {
		t.Fatalf("ListFacesMissingCropData: %v", err)
	}
	if len(missing) != 1 || missing[0].ID != faceNoCropID {
		t.Fatalf("want exactly [%d] missing crop data, got %+v", faceNoCropID, missing)
	}
	if missing[0].MediaItemID != noCropMediaItemID {
		t.Errorf("want media_item_id %d, got %d", noCropMediaItemID, missing[0].MediaItemID)
	}

	// SetFaceCropData (the backfill job's write path) — after this, the face
	// should both return its new data and drop out of the missing-list.
	backfilledCrop := []byte("fake-jpeg-bytes-backfilled")
	if err := repo.SetFaceCropData(ctx, faceNoCropID, backfilledCrop); err != nil {
		t.Fatalf("SetFaceCropData: %v", err)
	}
	got, err = repo.GetFaceCropData(ctx, faceNoCropID)
	if err != nil {
		t.Fatalf("GetFaceCropData (after backfill): %v", err)
	}
	if !bytes.Equal(got, backfilledCrop) {
		t.Fatalf("GetFaceCropData (after backfill): want %q, got %q", backfilledCrop, got)
	}

	missing, err = repo.ListFacesMissingCropData(ctx)
	if err != nil {
		t.Fatalf("ListFacesMissingCropData (after backfill): %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("want no faces missing crop data after backfill, got %+v", missing)
	}
}

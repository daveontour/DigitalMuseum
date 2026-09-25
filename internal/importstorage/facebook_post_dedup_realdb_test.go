package importstorage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// TestSavePostImagesBatch_ReimportDoesNotDuplicate reproduces re-running the
// "Facebook All" import over the same export directory a second time,
// against a real on-disk SQLite file (media_item_faces' cascade behavior
// only exists in the real migrations, not a hand-rolled schema). Covers two
// things that were broken before this fix: (1) SavePostImagesBatch had no
// dedup check at all, so every reimport duplicated every post photo; (2) its
// source_reference was PostID alone, which collides across every photo in a
// multi-photo post — this seeds a 2-photo post specifically to catch that.
// Confirms: the second import doesn't add rows, each photo keeps a distinct
// source_reference, post_media links are intact, and a media_item_faces row
// seeded on one photo (simulating an already-named face) survives untouched.
func TestSavePostImagesBatch_ReimportDoesNotDuplicate(t *testing.T) {
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

	postStorage := NewFacebookPostStorage(db.Std)
	ts := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	postID, created, err := postStorage.SaveOrUpdatePost(ctx, &ts, "Beach day", "had a great time", "", "")
	if err != nil {
		t.Fatalf("SaveOrUpdatePost: %v", err)
	}
	if !created {
		t.Fatal("want first SaveOrUpdatePost to create the post")
	}

	mkItems := func() []BatchPostImageItem {
		return []BatchPostImageItem{
			{PostID: postID, URI: "photos/beach1.jpg", Filename: "beach1.jpg", ImageData: []byte("a"), ImageType: "image/jpeg", PostTitle: "Beach day"},
			{PostID: postID, URI: "photos/beach2.jpg", Filename: "beach2.jpg", ImageData: []byte("b"), ImageType: "image/jpeg", PostTitle: "Beach day"},
		}
	}

	n1, err := postStorage.SavePostImagesBatch(ctx, mkItems())
	if err != nil {
		t.Fatalf("first SavePostImagesBatch: %v", err)
	}
	if n1 != 2 {
		t.Fatalf("first import: want 2 imported, got %d", n1)
	}

	var mediaItemIDs []int64
	var sourceRefs []string
	rows, err := db.Std.QueryContext(ctx, `SELECT id, source_reference FROM media_items WHERE source = 'facebook_post' ORDER BY id`)
	if err != nil {
		t.Fatalf("query media_items after first import: %v", err)
	}
	for rows.Next() {
		var id int64
		var ref string
		if err := rows.Scan(&id, &ref); err != nil {
			t.Fatalf("scan: %v", err)
		}
		mediaItemIDs = append(mediaItemIDs, id)
		sourceRefs = append(sourceRefs, ref)
	}
	_ = rows.Close()
	if len(mediaItemIDs) != 2 {
		t.Fatalf("want 2 media_items after first import, got %d", len(mediaItemIDs))
	}
	if sourceRefs[0] == sourceRefs[1] {
		t.Fatalf("want distinct source_reference per photo in a multi-photo post, both got %q", sourceRefs[0])
	}

	// Seed a face-recognition link on the first photo, simulating a person
	// already named via People in Photos before the reimport happens.
	var contactID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO contacts (name, user_id) VALUES ('Friend Name', ?1) RETURNING id`, uid).Scan(&contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	var clusterID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO face_clusters (contact_id, face_count, user_id, created_at, updated_at) VALUES (?1, 1, ?2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		contactID, uid).Scan(&clusterID); err != nil {
		t.Fatalf("seed face_cluster: %v", err)
	}
	var faceID int64
	confidence := 0.9
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO media_item_faces (media_item_id, bbox_x, bbox_y, bbox_w, bbox_h, detection_confidence, embedding_model, face_cluster_id, contact_id, user_id, created_at, updated_at)
		 VALUES (?1, 0.1, 0.1, 0.2, 0.2, ?2, 'test-model', ?3, ?4, ?5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		mediaItemIDs[0], confidence, clusterID, contactID, uid).Scan(&faceID); err != nil {
		t.Fatalf("seed media_item_faces: %v", err)
	}

	// Re-import the exact same post a second time (SaveOrUpdatePost reuses
	// the existing post row by timestamp+title, exactly as a second
	// "Facebook All" run over the same export would).
	postID2, created2, err := postStorage.SaveOrUpdatePost(ctx, &ts, "Beach day", "had a great time", "", "")
	if err != nil {
		t.Fatalf("second SaveOrUpdatePost: %v", err)
	}
	if created2 || postID2 != postID {
		t.Fatalf("want the second SaveOrUpdatePost to reuse post id %d, got id=%d created=%v", postID, postID2, created2)
	}

	n2, err := postStorage.SavePostImagesBatch(ctx, mkItems())
	if err != nil {
		t.Fatalf("second SavePostImagesBatch: %v", err)
	}
	t.Logf("second import: imported=%d (should just link existing photos)", n2)

	var mediaItemCount int
	if err := db.Std.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_items WHERE source = 'facebook_post'`).Scan(&mediaItemCount); err != nil {
		t.Fatalf("count media_items: %v", err)
	}
	if mediaItemCount != 2 {
		t.Errorf("want exactly 2 post-photo media_items after reimport, got %d", mediaItemCount)
	}

	var postMediaCount int
	if err := db.Std.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM post_media WHERE post_id = ?1`, postID).Scan(&postMediaCount); err != nil {
		t.Fatalf("count post_media: %v", err)
	}
	if postMediaCount != 2 {
		t.Errorf("want exactly 2 post_media links after reimport, got %d", postMediaCount)
	}

	var faceMediaItemID int64
	var faceContactID sql.NullInt64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT media_item_id, contact_id FROM media_item_faces WHERE id = ?1`, faceID).Scan(&faceMediaItemID, &faceContactID); err != nil {
		t.Fatalf("re-fetch seeded face after reimport: %v", err)
	}
	if faceMediaItemID != mediaItemIDs[0] {
		t.Errorf("want the seeded face to still point at media_item %d after reimport, got %d", mediaItemIDs[0], faceMediaItemID)
	}
	if !faceContactID.Valid || faceContactID.Int64 != contactID {
		t.Errorf("want the seeded face's contact link to survive the reimport, got %v", faceContactID)
	}
}

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

// TestSaveMessagesBatch_AttachmentReimportDoesNotDuplicate reproduces
// re-importing a message with an attached photo twice (as happens re-running
// a WhatsApp/iMessage/Facebook Messenger import over the same export) against
// a real on-disk SQLite file via database.New + database.MigrateSQLite (not a
// hand-rolled schema — media_item_faces' ON DELETE CASCADE only exists in the
// real migrations). Before this fix, saveAttachment always inserted a fresh
// media_items/media_blobs row for the attachment on every import, and the
// batch-update path additionally deleted the message_attachments junction
// row first — so a second import of the same export both duplicated the
// photo and orphaned (via the delete) any face-recognition data linked to
// the first import's copy. Confirms: exactly one media_items row exists for
// the attachment after two imports, and a media_item_faces row seeded
// against the first import's media_item id survives the second import
// completely untouched (same media_item_id, not cascaded away).
func TestSaveMessagesBatch_AttachmentReimportDoesNotDuplicate(t *testing.T) {
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

	storage := NewMessageStorage(ctx, db.Std, nil)

	chatSession := "Elena Marques"
	msgType := "Incoming"
	senderID := "+61467845299"
	text := "look at this"
	msgDate := time.Date(2026, 8, 1, 12, 5, 57, 0, time.UTC)

	mkMsg := func() MessageWithAttachment {
		return MessageWithAttachment{
			MessageData: MessageData{
				ChatSession: &chatSession,
				MessageDate: &msgDate,
				Service:     strPtr("WhatsApp"),
				Type:        &msgType,
				SenderID:    &senderID,
				SenderName:  &senderID,
				Status:      strPtr("Received"),
				Text:        &text,
			},
			AttachmentData:     []byte("fake-jpeg-bytes"),
			AttachmentFilename: "photo.jpg",
			AttachmentType:     "image/jpeg",
			Source:             "whatsapp",
		}
	}

	res1, err := storage.SaveMessagesBatch(ctx, []MessageWithAttachment{mkMsg()})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if res1.Created != 1 {
		t.Fatalf("first import: want 1 message created, got %d", res1.Created)
	}

	var mediaItemID int64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT id FROM media_items WHERE source = 'whatsapp' LIMIT 1`).Scan(&mediaItemID); err != nil {
		t.Fatalf("find attachment media_item after first import: %v", err)
	}

	// Seed a face-recognition link on that photo, simulating a person
	// already named via People in Photos before the reimport happens.
	var contactID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO contacts (name, user_id) VALUES ('Elena Marques', ?1) RETURNING id`, uid).Scan(&contactID); err != nil {
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
		mediaItemID, confidence, clusterID, contactID, uid).Scan(&faceID); err != nil {
		t.Fatalf("seed media_item_faces: %v", err)
	}

	// Re-import the exact same export a second time.
	res2, err := storage.SaveMessagesBatch(ctx, []MessageWithAttachment{mkMsg()})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	t.Logf("second import: created=%d updated=%d", res2.Created, res2.Updated)
	if res2.Created != 0 {
		t.Errorf("second import: want 0 messages created (should match existing), got %d", res2.Created)
	}

	var mediaItemCount int
	if err := db.Std.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_items WHERE source = 'whatsapp'`).Scan(&mediaItemCount); err != nil {
		t.Fatalf("count media_items: %v", err)
	}
	if mediaItemCount != 1 {
		t.Errorf("want exactly 1 attachment media_item after reimport, got %d", mediaItemCount)
	}

	var faceMediaItemID int64
	var faceContactID sql.NullInt64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT media_item_id, contact_id FROM media_item_faces WHERE id = ?1`, faceID).Scan(&faceMediaItemID, &faceContactID); err != nil {
		t.Fatalf("re-fetch seeded face after reimport: %v", err)
	}
	if faceMediaItemID != mediaItemID {
		t.Errorf("want the seeded face to still point at media_item %d after reimport, got %d", mediaItemID, faceMediaItemID)
	}
	if !faceContactID.Valid || faceContactID.Int64 != contactID {
		t.Errorf("want the seeded face's contact link to survive the reimport, got %v", faceContactID)
	}
}

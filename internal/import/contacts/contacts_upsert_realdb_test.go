package contacts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// TestRunContactsNormalise_UpsertPreservesExistingContacts reproduces the
// "Extract Contacts" flow against a real on-disk SQLite file (database.New +
// database.MigrateSQLite, not a hand-rolled schema, since media_item_faces'/
// face_clusters' ON DELETE SET NULL FK behavior only exists in the real
// migrations). Before this fix, RunContactsNormalise fully truncated and
// rebuilt the contacts table with freshly renumbered ids on every run,
// which — via those FKs — silently unlinked every face-to-person name
// across the whole archive and discarded any contact that had zero message
// history (e.g. one created purely to name a detected face).
//
// Seeds one existing contact that will be re-derived from email data this
// run (with a manually-set rel_type/description that must survive), one
// manually-created contact with zero messages (simulating the
// face-identify-and-create-contact UI flow) that must never be touched or
// deleted, and a media_item_faces/face_clusters row linked to the first
// contact that must keep pointing at the same contact id across two runs.
func TestRunContactsNormalise_UpsertPreservesExistingContacts(t *testing.T) {
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
	if _, err := db.Std.ExecContext(ctx,
		`INSERT INTO subject_configuration (subject_name, family_name, user_id) VALUES ('Dave', 'Burton', ?1)`,
		uid); err != nil {
		t.Fatalf("seed subject_configuration: %v", err)
	}
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)

	// Existing contact that email data below will re-derive and must match
	// onto (same id, merged fields), keeping its manually-set rel_type and
	// description untouched.
	var blakeID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO contacts (name, email, rel_type, description, user_id) VALUES ('Blake Whitney', 'blake@example.com', 'friend', 'met at university', ?1) RETURNING id`,
		uid).Scan(&blakeID); err != nil {
		t.Fatalf("seed existing contact: %v", err)
	}

	// Manually-created contact with zero message history, simulating a
	// contact created purely to name a detected face. Extraction has no
	// data that could ever match this, so it must survive untouched.
	var manualID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO contacts (name, rel_type, user_id) VALUES ('Manual Person', 'unknown', ?1) RETURNING id`,
		uid).Scan(&manualID); err != nil {
		t.Fatalf("seed manual contact: %v", err)
	}

	// A face-recognition link pointing at Blake Whitney's existing contact.
	var clusterID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO face_clusters (contact_id, face_count, user_id, created_at, updated_at) VALUES (?1, 1, ?2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		blakeID, uid).Scan(&clusterID); err != nil {
		t.Fatalf("seed face_cluster: %v", err)
	}
	var blobID int64
	if err := db.Std.QueryRowContext(ctx, `INSERT INTO media_blobs (image_data, user_id) VALUES (x'00', ?1) RETURNING id`, uid).Scan(&blobID); err != nil {
		t.Fatalf("seed media_blob: %v", err)
	}
	var mediaItemID int64
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, 'image/jpeg', ?2) RETURNING id`,
		blobID, uid).Scan(&mediaItemID); err != nil {
		t.Fatalf("seed media_item: %v", err)
	}
	var faceID int64
	confidence := 0.9
	if err := db.Std.QueryRowContext(ctx,
		`INSERT INTO media_item_faces (media_item_id, bbox_x, bbox_y, bbox_w, bbox_h, detection_confidence, embedding_model, face_cluster_id, contact_id, user_id, created_at, updated_at)
		 VALUES (?1, 0.1, 0.1, 0.2, 0.2, ?2, 'test-model', ?3, ?4, ?5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		mediaItemID, confidence, clusterID, blakeID, uid).Scan(&faceID); err != nil {
		t.Fatalf("seed media_item_faces: %v", err)
	}

	// Raw email data extraction reads from: one message to/from Blake
	// Whitney (should match the existing contact above), and one to/from a
	// brand-new person who has no existing contacts row.
	seedEmail := func(fromAddr string) {
		if _, err := db.Std.ExecContext(ctx,
			`INSERT INTO emails (uid, folder, from_address, to_addresses, user_id) VALUES (?1, 'INBOX', ?2, 'me@example.com', ?3)`,
			fromAddr, fromAddr, uid); err != nil {
			t.Fatalf("seed email from %q: %v", fromAddr, err)
		}
	}
	seedEmail("Blake Whitney <blake@example.com>")
	seedEmail("New Person <newperson@example.com>")

	runExtraction := func(label string) {
		t.Helper()
		opts := RunOptions{
			Workers:     4,
			ContactsDB:  db.Std,
			OwnerUserID: uid,
		}
		if err := RunContactsNormalise(ctx, opts); err != nil {
			t.Fatalf("%s: RunContactsNormalise: %v", label, err)
		}
	}

	runExtraction("first run")

	var afterFirstRelType, afterFirstDescription string
	if err := db.Std.QueryRowContext(ctx,
		`SELECT rel_type, COALESCE(description, '') FROM contacts WHERE id = ?1`, blakeID,
	).Scan(&afterFirstRelType, &afterFirstDescription); err != nil {
		t.Fatalf("re-fetch Blake after first run: %v", err)
	}
	if afterFirstRelType != "friend" || afterFirstDescription != "met at university" {
		t.Errorf("first run: want rel_type/description unchanged, got rel_type=%q description=%q", afterFirstRelType, afterFirstDescription)
	}

	var newPersonID int64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT id FROM contacts WHERE LOWER(email) LIKE '%newperson@example.com%'`,
	).Scan(&newPersonID); err != nil {
		t.Fatalf("find newly-inserted contact after first run: %v", err)
	}

	runExtraction("second run")

	// Blake Whitney's id, rel_type, and description must all survive a
	// second run unchanged.
	var afterSecondRelType, afterSecondDescription string
	if err := db.Std.QueryRowContext(ctx,
		`SELECT rel_type, COALESCE(description, '') FROM contacts WHERE id = ?1`, blakeID,
	).Scan(&afterSecondRelType, &afterSecondDescription); err != nil {
		t.Fatalf("re-fetch Blake after second run: %v", err)
	}
	if afterSecondRelType != "friend" || afterSecondDescription != "met at university" {
		t.Errorf("second run: want rel_type/description still unchanged, got rel_type=%q description=%q", afterSecondRelType, afterSecondDescription)
	}

	// The manually-created, zero-message contact must still exist,
	// completely untouched, after two extraction runs.
	var manualStillExists int
	if err := db.Std.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contacts WHERE id = ?1 AND name = 'Manual Person'`, manualID,
	).Scan(&manualStillExists); err != nil {
		t.Fatalf("check manual contact after second run: %v", err)
	}
	if manualStillExists != 1 {
		t.Errorf("want the manually-created zero-message contact to survive two extraction runs, got count=%d", manualStillExists)
	}

	// The new person's id must be stable across the second run too (not
	// renumbered).
	var newPersonIDAfterSecond int64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT id FROM contacts WHERE LOWER(email) LIKE '%newperson@example.com%'`,
	).Scan(&newPersonIDAfterSecond); err != nil {
		t.Fatalf("find new-person contact after second run: %v", err)
	}
	if newPersonIDAfterSecond != newPersonID {
		t.Errorf("want new-person contact id stable across runs, first=%d second=%d", newPersonID, newPersonIDAfterSecond)
	}

	// The face-recognition link seeded against Blake Whitney's contact must
	// still point at the same contact id, and the face itself (bbox/cluster
	// membership) must be completely untouched.
	var faceContactID sql.NullInt64
	var faceMediaItemID int64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT contact_id, media_item_id FROM media_item_faces WHERE id = ?1`, faceID,
	).Scan(&faceContactID, &faceMediaItemID); err != nil {
		t.Fatalf("re-fetch seeded face after second run: %v", err)
	}
	if !faceContactID.Valid || faceContactID.Int64 != blakeID {
		t.Errorf("want the seeded face still linked to contact %d after two extraction runs, got %v", blakeID, faceContactID)
	}
	if faceMediaItemID != mediaItemID {
		t.Errorf("want the seeded face's media_item_id unchanged, got %d want %d", faceMediaItemID, mediaItemID)
	}

	var clusterContactID sql.NullInt64
	if err := db.Std.QueryRowContext(ctx,
		`SELECT contact_id FROM face_clusters WHERE id = ?1`, clusterID,
	).Scan(&clusterContactID); err != nil {
		t.Fatalf("re-fetch seeded cluster after second run: %v", err)
	}
	if !clusterContactID.Valid || clusterContactID.Int64 != blakeID {
		t.Errorf("want the seeded cluster still linked to contact %d after two extraction runs, got %v", blakeID, clusterContactID)
	}
}

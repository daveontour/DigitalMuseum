package importstorage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupMessagesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
	CREATE TABLE messages (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_session   VARCHAR(500),
		message_date   TIMESTAMP,
		is_group_chat  BOOLEAN NOT NULL DEFAULT FALSE,
		delivered_date TIMESTAMP,
		read_date      TIMESTAMP,
		edited_date    TIMESTAMP,
		service        VARCHAR(100),
		type           VARCHAR(50),
		sender_id      VARCHAR(255),
		sender_name    VARCHAR(500),
		status         VARCHAR(100),
		replying_to    VARCHAR(500),
		subject        VARCHAR(1000),
		text           TEXT,
		processed      BOOLEAN NOT NULL DEFAULT FALSE,
		created_at     TIMESTAMP,
		updated_at     TIMESTAMP,
		user_id        BIGINT
	);
	CREATE TABLE media_blobs (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		image_data     BLOB,
		thumbnail_data BLOB,
		user_id        BIGINT
	);
	CREATE TABLE media_items (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		media_blob_id      INTEGER,
		tags               TEXT,
		source             VARCHAR(100),
		source_reference   VARCHAR(500),
		title              VARCHAR(500),
		description        TEXT,
		media_type         VARCHAR(100),
		year               INTEGER,
		month              INTEGER,
		latitude           REAL,
		longitude          REAL,
		altitude           REAL,
		has_gps            BOOLEAN,
		processed          BOOLEAN,
		available_for_task BOOLEAN,
		rating             INTEGER,
		is_personal        BOOLEAN,
		is_business        BOOLEAN,
		is_social          BOOLEAN,
		is_promotional     BOOLEAN,
		is_spam            BOOLEAN,
		is_important       BOOLEAN,
		user_id            BIGINT,
		created_at         TIMESTAMP,
		updated_at         TIMESTAMP,
		is_referenced      BOOLEAN
	);
	CREATE TABLE message_attachments (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id    INTEGER,
		media_item_id INTEGER,
		user_id       BIGINT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// TestSaveMessagesBatch_ReimportDoesNotDuplicate reproduces importing the same
// CSV-derived batch twice (as happens when a user re-runs an iMessage/SMS
// import over the same folder) and asserts the second run does not create
// duplicate rows.
func TestSaveMessagesBatch_ReimportDoesNotDuplicate(t *testing.T) {
	db := setupMessagesDB(t)
	storage := NewMessageStorage(context.Background(), db, nil)

	mkMsg := func(chatSession, senderID, msgType, text string, date time.Time) MessageWithAttachment {
		return MessageWithAttachment{
			MessageData: MessageData{
				ChatSession: &chatSession,
				MessageDate: &date,
				Service:     strPtr("iMessage"),
				Type:        &msgType,
				SenderID:    &senderID,
				SenderName:  &senderID,
				Status:      strPtr("Received"),
				Text:        &text,
			},
		}
	}

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	batch := []MessageWithAttachment{
		mkMsg("Alice", "+15551234567", "Incoming", "hello", base),
		mkMsg("Alice", "+15551234567", "Incoming", "how are you", base.Add(1*time.Second)),
		mkMsg("Alice", "Dave", "Outgoing", "good thanks", base.Add(2*time.Second)),
	}

	ctx := context.Background()

	res1, err := storage.SaveMessagesBatch(ctx, batch)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if res1.Created != 3 {
		t.Fatalf("first import: want 3 created, got %d (updated=%d errors=%d)", res1.Created, res1.Updated, res1.Errors)
	}

	// Re-import the exact same batch, simulating the user re-selecting the
	// same folder a second time.
	batch2 := []MessageWithAttachment{
		mkMsg("Alice", "+15551234567", "Incoming", "hello", base),
		mkMsg("Alice", "+15551234567", "Incoming", "how are you", base.Add(1*time.Second)),
		mkMsg("Alice", "Dave", "Outgoing", "good thanks", base.Add(2*time.Second)),
	}
	res2, err := storage.SaveMessagesBatch(ctx, batch2)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if res2.Created != 0 {
		t.Errorf("second import: want 0 created (all should match existing rows), got %d created, %d updated", res2.Created, res2.Updated)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("want 3 rows in messages table after reimport, got %d", count)
	}
}

func strPtr(s string) *string { return &s }

// TestSaveMessagesBatch_OutgoingSenderIDDependsOnSubjectName covers a
// duplicate-on-reimport scenario: Outgoing messages get their sender_id
// rewritten to the *current* subject full name on every import. If the
// subject's configured name is empty on the first import (e.g. the archive
// owner hasn't filled in Subject Configuration yet) and set to a real name
// by the time the same folder is re-imported, sender_id must not be part of
// the "already exists" lookup key for Outgoing messages — otherwise the
// changed value would cause every Outgoing message to be re-inserted as a
// duplicate instead of being recognized as already imported.
func TestSaveMessagesBatch_OutgoingSenderIDDependsOnSubjectName(t *testing.T) {
	db := setupMessagesDB(t)

	mkMsg := func(chatSession, senderID, msgType, text string, date time.Time) MessageWithAttachment {
		return MessageWithAttachment{
			MessageData: MessageData{
				ChatSession: &chatSession,
				MessageDate: &date,
				Service:     strPtr("iMessage"),
				Type:        &msgType,
				SenderID:    &senderID,
				SenderName:  &senderID,
				Status:      strPtr("Sent"),
				Text:        &text,
			},
		}
	}

	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// Raw CSV Sender ID is blank for the owner's own outgoing texts, which is
	// typical for iMazing exports.
	batch := []MessageWithAttachment{
		mkMsg("Alice", "", "Outgoing", "good thanks", base),
	}

	ctx := context.Background()

	// First import: subject full name not yet configured.
	storage1 := &MessageStorage{pool: db, subjectFullName: ""}
	res1, err := storage1.SaveMessagesBatch(ctx, batch)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if res1.Created != 1 {
		t.Fatalf("first import: want 1 created, got %d", res1.Created)
	}

	// Re-import the exact same folder after the user has since filled in
	// their name in Subject Configuration.
	batch2 := []MessageWithAttachment{
		mkMsg("Alice", "", "Outgoing", "good thanks", base),
	}
	storage2 := &MessageStorage{pool: db, subjectFullName: "Dave Burton"}
	res2, err := storage2.SaveMessagesBatch(ctx, batch2)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}

	t.Logf("second import: created=%d updated=%d, total rows=%d", res2.Created, res2.Updated, count)
	if res2.Created != 0 || res2.Updated != 1 {
		t.Errorf("want the reimport to update the existing row (created=0, updated=1), got created=%d updated=%d", res2.Created, res2.Updated)
	}
	if count != 1 {
		t.Errorf("want 1 row after reimport (message should be recognized as already existing), got %d", count)
	}

	// The subject's newer name should still be reflected (sender_name is
	// refreshed on update even though it's excluded from the dedup key).
	var senderName string
	if err := db.QueryRow("SELECT sender_name FROM messages LIMIT 1").Scan(&senderName); err != nil {
		t.Fatalf("query sender_name: %v", err)
	}
	if senderName != "Dave Burton" {
		t.Errorf("want sender_name updated to the current subject name %q, got %q", "Dave Burton", senderName)
	}
}

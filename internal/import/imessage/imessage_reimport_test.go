package imessage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/daveontour/aimuseum/internal/importstorage"
)

func setupImessageDB(t *testing.T) *sql.DB {
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

// TestImportIMessagesFromDirectory_ReimportDoesNotDuplicate drives the full,
// real import entry point (ImportIMessagesFromDirectory) against an on-disk
// folder structure exactly as produced by an iMazing-style export — a parent
// directory containing one subdirectory per conversation, each with a CSV
// file and any attachment/orphan image files — and runs it twice over the
// same directory, as a user does when re-importing to pick up new messages.
// It asserts the second run does not create duplicate rows for any message
// type (Incoming, Outgoing) or for orphan (unreferenced) image attachments.
func TestImportIMessagesFromDirectory_ReimportDoesNotDuplicate(t *testing.T) {
	root := t.TempDir()
	convDir := filepath.Join(root, "Alice")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A real attachment referenced by the CSV row.
	if err := os.WriteFile(filepath.Join(convDir, "photo.jpg"), []byte("fakejpegdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An orphan image NOT referenced by any CSV row (e.g. a device photo
	// iMazing exported into the folder that isn't tied to a specific text).
	if err := os.WriteFile(filepath.Join(convDir, "orphan.jpg"), []byte("orphandata"), 0o644); err != nil {
		t.Fatal(err)
	}

	csvContent := "" +
		"Message Date,Delivered Date,Read Date,Edited Date,Chat Session,Service,Type,Sender ID,Sender Name,Status,Replying to,Subject,Text,Attachment,Attachment type\n" +
		"2024-01-01 12:00:00,,,,\"Alice\",iMessage,Incoming,+15551234567,Alice,Received,,,hello there,,\n" +
		"2024-01-01 12:00:05,,,,\"Alice\",iMessage,Outgoing,,,Sent,,,hi Alice!,,\n" +
		"2024-01-01 12:00:10,,,,\"Alice\",iMessage,Incoming,+15551234567,Alice,Received,,,here's a pic,photo.jpg,image/jpeg\n"

	if err := os.WriteFile(filepath.Join(convDir, "messages.csv"), []byte(csvContent), 0o644); err != nil {
		t.Fatal(err)
	}

	db := setupImessageDB(t)
	ctx := context.Background()
	storage := importstorage.NewMessageStorage(ctx, db, nil)

	runImport := func() *ImportStats {
		stats, err := ImportIMessagesFromDirectory(ctx, storage, root, nil, nil)
		if err != nil {
			t.Fatalf("ImportIMessagesFromDirectory: %v", err)
		}
		return stats
	}

	stats1 := runImport()
	if stats1.MessagesCreated == 0 {
		t.Fatalf("first import created 0 messages, expected at least 3 (got stats: %+v)", stats1)
	}

	var countAfterFirst int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&countAfterFirst); err != nil {
		t.Fatalf("count after first import: %v", err)
	}

	// Re-import the exact same directory, simulating the user re-selecting
	// the same folder a second time to pick up any new messages.
	stats2 := runImport()

	var countAfterSecond int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&countAfterSecond); err != nil {
		t.Fatalf("count after second import: %v", err)
	}

	t.Logf("first import: created=%d updated=%d errors=%d attachments_found=%d orphan_imports=%d",
		stats1.MessagesCreated, stats1.MessagesUpdated, stats1.Errors, stats1.AttachmentsFound, stats1.OrphanAttachmentsImported)
	t.Logf("second import: created=%d updated=%d errors=%d attachments_found=%d orphan_imports=%d",
		stats2.MessagesCreated, stats2.MessagesUpdated, stats2.Errors, stats2.AttachmentsFound, stats2.OrphanAttachmentsImported)
	t.Logf("message row count: after first=%d after second=%d", countAfterFirst, countAfterSecond)

	if stats2.MessagesCreated != 0 {
		t.Errorf("second import: want 0 newly created messages, got %d created (%d updated)", stats2.MessagesCreated, stats2.MessagesUpdated)
	}
	if countAfterSecond != countAfterFirst {
		t.Errorf("want message row count unchanged after reimport: after first=%d, after second=%d", countAfterFirst, countAfterSecond)
	}

	// Print every row for inspection if the test fails, to see exactly what
	// differs between the "duplicate" rows.
	if t.Failed() {
		rows, err := db.Query("SELECT id, chat_session, message_date, type, sender_id, sender_name, text FROM messages ORDER BY id")
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var id int64
				var chatSession, msgType, senderID, senderName, text string
				var msgDate any
				if err := rows.Scan(&id, &chatSession, &msgDate, &msgType, &senderID, &senderName, &text); err == nil {
					t.Logf("row id=%d chat_session=%q message_date=%v type=%q sender_id=%q sender_name=%q text=%q",
						id, chatSession, msgDate, msgType, senderID, senderName, text)
				}
			}
		}
	}
}

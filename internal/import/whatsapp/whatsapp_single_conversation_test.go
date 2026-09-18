package whatsapp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/daveontour/aimuseum/internal/importstorage"
)

func setupWhatsAppDB(t *testing.T) *sql.DB {
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
		message_date   TEXT,
		is_group_chat  BOOLEAN NOT NULL DEFAULT FALSE,
		delivered_date TEXT,
		read_date      TEXT,
		edited_date    TEXT,
		service        VARCHAR(100),
		type           VARCHAR(50),
		sender_id      VARCHAR(255),
		sender_name    VARCHAR(500),
		status         VARCHAR(100),
		replying_to    VARCHAR(500),
		subject        VARCHAR(1000),
		text           TEXT,
		processed      BOOLEAN NOT NULL DEFAULT FALSE,
		created_at     TEXT,
		updated_at     TEXT,
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
		created_at         TEXT,
		updated_at         TEXT,
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

const whatsappCSVHeader = "Message Date,Sent Date,Chat Session,Type,Sender ID,Sender Name,Status,Replying to,Text,Attachment,Attachment type\n"

// TestImportWhatsAppFromDirectory_SingleConversationFolder mirrors the
// iMessage/SMS fix: selecting a single conversation folder directly (one
// that contains a CSV file itself, with no per-conversation subdirectories)
// must import that conversation instead of reporting zero conversations
// found, matching the behavior of selecting the folder's parent.
func TestImportWhatsAppFromDirectory_SingleConversationFolder(t *testing.T) {
	convDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(convDir, "photo.jpg"), []byte("fakejpegdata"), 0o644); err != nil {
		t.Fatal(err)
	}

	csvContent := whatsappCSVHeader +
		"2024-01-01 12:00:00,2024-01-01 12:00:00,\"Alice\",Incoming,+15551234567,Alice,Received,,hello there,,\n" +
		"2024-01-01 12:00:05,2024-01-01 12:00:05,\"Alice\",Outgoing,,,Sent,,hi Alice!,,\n" +
		"2024-01-01 12:00:10,2024-01-01 12:00:10,\"Alice\",Incoming,+15551234567,Alice,Received,,here's a pic,photo.jpg,image/jpeg\n"

	if err := os.WriteFile(filepath.Join(convDir, "messages.csv"), []byte(csvContent), 0o644); err != nil {
		t.Fatal(err)
	}

	db := setupWhatsAppDB(t)
	ctx := context.Background()
	storage := importstorage.NewMessageStorage(ctx, db, nil)

	// Select the conversation folder itself (the folder the messages are
	// actually in), not its parent.
	stats, err := ImportWhatsAppFromDirectory(ctx, storage, convDir, nil, nil)
	if err != nil {
		t.Fatalf("ImportWhatsAppFromDirectory: %v", err)
	}

	if stats.TotalConversations != 1 {
		t.Errorf("want 1 conversation detected, got %d", stats.TotalConversations)
	}
	if stats.MessagesCreated != 3 {
		t.Errorf("want 3 messages created, got %d (errors=%d)", stats.MessagesCreated, stats.Errors)
	}
	if stats.AttachmentsFound != 1 {
		t.Errorf("want 1 attachment found, got %d (missing=%d)", stats.AttachmentsFound, stats.AttachmentsMissing)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("want 3 rows in messages table, got %d", count)
	}

	var attachCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM message_attachments").Scan(&attachCount); err != nil {
		t.Fatalf("attach count: %v", err)
	}
	if attachCount != 1 {
		t.Errorf("want 1 message_attachments row, got %d", attachCount)
	}
}

// TestImportWhatsAppFromDirectory_ParentFolderStillWorks guards against a
// regression in the normal (parent-folder) case while adding the
// single-conversation-folder support above.
func TestImportWhatsAppFromDirectory_ParentFolderStillWorks(t *testing.T) {
	root := t.TempDir()
	convDir := filepath.Join(root, "Alice")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	csvContent := whatsappCSVHeader +
		"2024-01-01 12:00:00,2024-01-01 12:00:00,\"Alice\",Incoming,+15551234567,Alice,Received,,hello there,,\n"

	if err := os.WriteFile(filepath.Join(convDir, "messages.csv"), []byte(csvContent), 0o644); err != nil {
		t.Fatal(err)
	}

	db := setupWhatsAppDB(t)
	ctx := context.Background()
	storage := importstorage.NewMessageStorage(ctx, db, nil)

	stats, err := ImportWhatsAppFromDirectory(ctx, storage, root, nil, nil)
	if err != nil {
		t.Fatalf("ImportWhatsAppFromDirectory: %v", err)
	}
	if stats.TotalConversations != 1 {
		t.Errorf("want 1 conversation detected, got %d", stats.TotalConversations)
	}
	if stats.MessagesCreated != 1 {
		t.Errorf("want 1 message created, got %d (errors=%d)", stats.MessagesCreated, stats.Errors)
	}
}

package imessage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/importstorage"
)

// TestImportIMessagesFromDirectory_ReimportDoesNotDuplicate_RealDB is the
// highest-fidelity reproduction available in a unit test: it opens an actual
// on-disk SQLite file through database.New (same connection pool settings —
// MaxOpenConns(1) — and the same DSN, "file:<path>?_foreign_keys=1&_busy_timeout=5000",
// that the real app uses) and applies the real database.MigrateSQLite schema
// (not a hand-rolled subset), instead of the simplified in-memory schema used
// by the sibling test in this package. This rules out any difference between
// that simplified test and production being the reason duplicates were still
// observed after the SaveMessagesBatch dedup-key fix.
func TestImportIMessagesFromDirectory_ReimportDoesNotDuplicate_RealDB(t *testing.T) {
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

	convDir := filepath.Join(dir, "import", "Elena Marques")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	csvContent := "" +
		"Message Date,Delivered Date,Read Date,Edited Date,Chat Session,Service,Type,Sender ID,Sender Name,Status,Replying to,Subject,Text,Attachment,Attachment type\n" +
		"2026-08-01 12:05:57,,,,\"Elena Marques\",iMessage,Incoming,+61467845299,Elena Marques,Received,,,haha that's funny,,\n" +
		"2026-06-13 15:37:06,,,,\"Elena Marques\",iMessage,Outgoing,,,Sent,,,\"On the way out I had the wind behind me\",,\n" +
		"2026-06-26 19:18:17,,,,\"Elena Marques\",iMessage,Outgoing,,,Sent,,,You are definitely affectionate with me,,\n"

	if err := os.WriteFile(filepath.Join(convDir, "messages.csv"), []byte(csvContent), 0o644); err != nil {
		t.Fatal(err)
	}

	storage := importstorage.NewMessageStorage(ctx, db.Std, nil)

	runImport := func(label string) *ImportStats {
		stats, err := ImportIMessagesFromDirectory(ctx, storage, filepath.Join(dir, "import"), nil, nil)
		if err != nil {
			t.Fatalf("%s: ImportIMessagesFromDirectory: %v", label, err)
		}
		return stats
	}

	stats1 := runImport("first import")
	var countAfterFirst int
	if err := db.Std.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages").Scan(&countAfterFirst); err != nil {
		t.Fatalf("count after first import: %v", err)
	}
	t.Logf("first import: created=%d updated=%d errors=%d, row count=%d", stats1.MessagesCreated, stats1.MessagesUpdated, stats1.Errors, countAfterFirst)
	if countAfterFirst != 3 {
		t.Fatalf("want 3 rows after first import, got %d", countAfterFirst)
	}

	// Re-open the storage against the same *sql.DB the way a second,
	// separate "Start Import" click from the UI would: a fresh
	// MessageStorage instance re-reading subject config, exactly as
	// runIMessageInProcess does per request.
	storage2 := importstorage.NewMessageStorage(ctx, db.Std, nil)
	stats2, err := ImportIMessagesFromDirectory(ctx, storage2, filepath.Join(dir, "import"), nil, nil)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	var countAfterSecond int
	if err := db.Std.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages").Scan(&countAfterSecond); err != nil {
		t.Fatalf("count after second import: %v", err)
	}
	t.Logf("second import: created=%d updated=%d errors=%d, row count=%d", stats2.MessagesCreated, stats2.MessagesUpdated, stats2.Errors, countAfterSecond)

	if stats2.MessagesCreated != 0 {
		t.Errorf("second import: want 0 created, got %d created (%d updated)", stats2.MessagesCreated, stats2.MessagesUpdated)
	}
	if countAfterSecond != countAfterFirst {
		t.Errorf("BUG: row count changed after reimport: first=%d second=%d", countAfterFirst, countAfterSecond)
	}

	if t.Failed() {
		rows, qerr := db.Std.QueryContext(ctx, "SELECT id, chat_session, sender_id, message_date, type, text FROM messages ORDER BY text, id")
		if qerr == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var id int64
				var chatSession, senderID, msgType, text string
				var msgDate any
				if err := rows.Scan(&id, &chatSession, &senderID, &msgDate, &msgType, &text); err == nil {
					t.Logf("row id=%d chat_session=%q sender_id=%q message_date=%v(%T) type=%q text=%q",
						id, chatSession, senderID, msgDate, msgDate, msgType, text)
				}
			}
		}
	}
}

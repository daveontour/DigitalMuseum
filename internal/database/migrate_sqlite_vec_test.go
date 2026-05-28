package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func TestEnsureSQLiteVecEmbeddingTables_CreatesAndIsIdempotent(t *testing.T) {
	t.Helper()

	sqlite_vec.Auto()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureSQLiteVecEmbeddingTables(ctx, db); err != nil {
		t.Fatalf("first ensureSQLiteVecEmbeddingTables: %v", err)
	}
	if err := ensureSQLiteVecEmbeddingTables(ctx, db); err != nil {
		t.Fatalf("second ensureSQLiteVecEmbeddingTables: %v", err)
	}

	for _, table := range []string{
		"email_embeddings",
		"message_embeddings",
		"media_tag_embeddings",
		"facebook_post_text_embeddings",
		"facebook_album_description_embeddings",
		"artefact_embeddings",
	} {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master %s: %v", table, err)
		}
		if n != 1 {
			t.Fatalf("expected %s to exist exactly once, got %d", table, n)
		}

		var ddl string
		if err := db.QueryRowContext(ctx,
			`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&ddl); err != nil {
			t.Fatalf("sqlite_master sql %s: %v", table, err)
		}
		if !strings.Contains(strings.ToLower(ddl), "vec0") {
			t.Fatalf("%s should be vec0 virtual table, ddl=%q", table, ddl)
		}
		if !strings.Contains(strings.ToLower(ddl), "embedding float[768]") {
			t.Fatalf("%s missing embedding float[768], ddl=%q", table, ddl)
		}
		if !strings.Contains(strings.ToLower(ddl), "int_ids text") {
			t.Fatalf("%s missing int_ids text, ddl=%q", table, ddl)
		}
	}
}

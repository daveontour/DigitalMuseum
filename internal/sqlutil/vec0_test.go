package sqlutil

import (
	"context"
	"database/sql"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func TestVec0Upsert_ReplacesExistingRow(t *testing.T) {
	t.Helper()
	sqlite_vec.Auto()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE media_tag_embeddings USING vec0(embedding float[768], int_ids text)`); err != nil {
		t.Fatalf("create vec0: %v", err)
	}

	vec := make([]float32, 768)
	blob1, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	vec[0] = 1.0
	blob2, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	const table = "media_tag_embeddings"
	if err := Vec0Upsert(ctx, db, table, 42, blob1, "sig-a"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := Vec0Upsert(ctx, db, table, 42, blob2, "sig-b"); err != nil {
		t.Fatalf("second upsert (replace): %v", err)
	}

	var got string
	if err := db.QueryRowContext(ctx, `SELECT int_ids FROM media_tag_embeddings WHERE rowid = 42`).Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if got != "sig-b" {
		t.Fatalf("int_ids = %q; want sig-b", got)
	}
}

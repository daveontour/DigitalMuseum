package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// TestPathEquivalenceRepo_RealDB exercises PathEquivalenceRepo's CRUD against
// a real on-disk SQLite file via the actual database.New + database.MigrateSQLite
// path (not a hand-rolled test schema — see TestFaceRepo_RealDB's comment on
// why this matters in this codebase), confirming create/list/update/delete
// round-trip correctly and are scoped to the authenticated user.
func TestPathEquivalenceRepo_RealDB(t *testing.T) {
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
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'filesystem_path_equivalences'`,
	).Scan(&n); err != nil {
		t.Fatalf("sqlite_master lookup: %v", err)
	}
	if n == 0 {
		t.Fatal("expected filesystem_path_equivalences table to exist after migration")
	}

	uidA := int64(2)
	uidB := int64(3)
	for _, uid := range []int64{uidA, uidB} {
		if _, err := db.Std.ExecContext(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, ?2, 'x', 'Test', 1, 0)`,
			uid, fmt.Sprintf("user%d@b.c", uid),
		); err != nil {
			t.Fatalf("seed user %d: %v", uid, err)
		}
	}

	repo := NewPathEquivalenceRepo(db.Std)
	ctxA := context.WithValue(ctx, appctx.ContextKeyUserID, uidA)
	ctxB := context.WithValue(ctx, appctx.ContextKeyUserID, uidB)

	created, err := repo.Create(ctxA, `C:\NonOneDrive\iCloud Sorted`, `D:\NonOneDrive\iCloud Sorted`)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("want a non-zero id")
	}

	// A second user's rule list must not see the first user's rule.
	if _, err := repo.Create(ctxB, `E:\Other`, `F:\Other`); err != nil {
		t.Fatalf("Create (user B): %v", err)
	}

	listA, err := repo.List(ctxA)
	if err != nil {
		t.Fatalf("List (user A): %v", err)
	}
	if len(listA) != 1 || listA[0].ID != created.ID {
		t.Fatalf("want exactly user A's own rule, got %+v", listA)
	}

	listB, err := repo.List(ctxB)
	if err != nil {
		t.Fatalf("List (user B): %v", err)
	}
	if len(listB) != 1 || listB[0].PathA != `E:\Other` {
		t.Fatalf("want exactly user B's own rule, got %+v", listB)
	}

	// Update.
	updated, err := repo.Update(ctxA, created.ID, `C:\Moved`, `D:\Moved`)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || updated.PathA != `C:\Moved` || updated.PathB != `D:\Moved` {
		t.Fatalf("want updated paths, got %+v", updated)
	}

	// User B must not be able to update user A's rule.
	crossUpdate, err := repo.Update(ctxB, created.ID, `X:\Nope`, `Y:\Nope`)
	if err != nil {
		t.Fatalf("Update (cross-user): %v", err)
	}
	if crossUpdate != nil {
		t.Fatalf("want cross-user update to no-op (nil), got %+v", crossUpdate)
	}

	// User B must not be able to delete user A's rule.
	deletedCross, err := repo.Delete(ctxB, created.ID)
	if err != nil {
		t.Fatalf("Delete (cross-user): %v", err)
	}
	if deletedCross {
		t.Fatal("want cross-user delete to report false (not found for this user)")
	}

	// Delete for real, by the owning user.
	deleted, err := repo.Delete(ctxA, created.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Fatal("want delete to succeed for the owning user")
	}

	listAAfter, err := repo.List(ctxA)
	if err != nil {
		t.Fatalf("List (user A, after delete): %v", err)
	}
	if len(listAAfter) != 0 {
		t.Fatalf("want empty list after delete, got %+v", listAAfter)
	}
}

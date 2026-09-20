package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// TestContactRepo_ListNames_IncludesContactsWithNoMessages guards against a
// regression where ListNames (backing GET /contacts/names, used by several
// "pick any contact" pickers — linking a face-recognition cluster, selecting
// contacts for a Profile) required at least one message/email before a
// contact would appear at all. A contact created purely to name a face (via
// "Add ... as a new person" in People in Photos, or added manually) has zero
// message counts and must still be listed.
func TestContactRepo_ListNames_IncludesContactsWithNoMessages(t *testing.T) {
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
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)
	if _, err := db.Std.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, 'a@b.c', 'x', 'Test', 1, 0)`,
		uid,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	repo := NewContactRepo(db.Std)

	// A contact with zero message/email counts (e.g. created solely to name a
	// face-recognition cluster) — no numemails/numsms/etc. columns set.
	created, err := repo.CreateContact(ctx, "Blake Whitney")
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	// A phone-number-only name must still be excluded (unrelated to message counts).
	if _, err := db.Std.ExecContext(ctx, `INSERT INTO contacts (name, user_id) VALUES ('+1 555 123 4567', ?1)`, uid); err != nil {
		t.Fatalf("seed phone-number contact: %v", err)
	}

	names, err := repo.ListNames(ctx)
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}

	found := false
	for _, n := range names {
		if n.Name == "+1 555 123 4567" {
			t.Errorf("want phone-number-only name excluded from ListNames, got it: %+v", n)
		}
		if n.ID == created.ID {
			found = true
			if n.Name != "Blake Whitney" {
				t.Errorf("want name %q, got %q", "Blake Whitney", n.Name)
			}
		}
	}
	if !found {
		t.Fatalf("want zero-message contact %q (id=%d) included in ListNames, got %+v", "Blake Whitney", created.ID, names)
	}
}

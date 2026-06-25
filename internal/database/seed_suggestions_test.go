package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/suggestions"
	_ "github.com/mattn/go-sqlite3"
)

func TestSeedSuggestionsFromFileIfMissingInsertOnly(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, stmt := range []string{
		`CREATE TABLE suggestions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL,
			text TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX uq_suggestions_key ON suggestions (key)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "suggestions.json")
	initial := `{"categories":[{"category":"Cat A","suggestions":[
		{"title":"One","prompt":"Prompt one"}
	]}]}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SeedSuggestionsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	key, _ := suggestions.BuildKey("Cat A", "One")
	var text string
	if err := db.QueryRowContext(ctx, `SELECT text FROM suggestions WHERE key = ?`, key).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != `{"prompt":"Prompt one","title":"One"}` && text != `{"title":"One","prompt":"Prompt one"}` {
		t.Fatalf("initial text = %s", text)
	}

	updated := `{"categories":[{"category":"Cat A","suggestions":[
		{"title":"One","prompt":"Changed prompt"},
		{"title":"Two","prompt":"Prompt two"}
	]}]}`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedSuggestionsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT text FROM suggestions WHERE key = ?`, key).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text == `{"prompt":"Changed prompt","title":"One"}` || text == `{"title":"One","prompt":"Changed prompt"}` {
		t.Fatalf("existing row was overwritten: %s", text)
	}

	keyTwo, _ := suggestions.BuildKey("Cat A", "Two")
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM suggestions WHERE key = ?`, keyTwo).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("new key count = %d, want 1", count)
	}
}

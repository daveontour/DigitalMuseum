package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/georegion"
	_ "github.com/mattn/go-sqlite3"
)

func TestSeedRegionsFromFileIfMissingInsertOnly(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE regions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			text TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX uq_regions_key ON regions (key)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "regions.json")
	initial := `{"default_region":"oth","default_label":"Other","regions":[
		{"code":"aus","label":"Australia","bbox":[110,-44,152,-10]}
	]}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SeedRegionsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM regions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("after first seed count = %d, want 3", count)
	}

	updated := `{"default_region":"xx","default_label":"Unknown","regions":[
		{"code":"aus","label":"Changed","bbox":[1,2,3,4]},
		{"code":"newr","label":"New","bbox":[5,6,7,8]}
	]}`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SeedRegionsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM regions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("after second seed count = %d, want 4 (insert-if-missing only)", count)
	}

	var ausText string
	if err := db.QueryRowContext(ctx, `SELECT text FROM regions WHERE key = 'aus'`).Scan(&ausText); err != nil {
		t.Fatal(err)
	}
	if ausText != `{"code":"aus","label":"Australia","bbox":[110,-44,152,-10]}` {
		t.Fatalf("existing aus row was overwritten: %s", ausText)
	}

	var defaultRegion string
	if err := db.QueryRowContext(ctx, `SELECT text FROM regions WHERE key = ?`, georegion.KeyDefaultRegion).Scan(&defaultRegion); err != nil {
		t.Fatal(err)
	}
	if defaultRegion != `"oth"` {
		t.Fatalf("default region row = %s, want unchanged", defaultRegion)
	}

	var newCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM regions WHERE key = 'newr'`).Scan(&newCount); err != nil {
		t.Fatal(err)
	}
	if newCount != 1 {
		t.Fatalf("new key newr count = %d, want 1", newCount)
	}
}

package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestSeedGuideTopicsFromFileIfMissingInsertOnly(t *testing.T) {
	ctx := context.Background()
	db := openGuideTopicsTestDB(t, ctx)
	defer db.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "guide_topics.json")
	writeGuideTopicsSeedFile(t, path, `{"topics":[
		{"key":"Alpha","title":"Alpha title","category":"Daily Use","steps":[]}
	]}`)

	if err := SeedGuideTopicsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}

	writeGuideTopicsSeedFile(t, path, `{"topics":[
		{"key":"Alpha","title":"Changed title","category":"Daily Use","steps":[]},
		{"key":"Beta","title":"Beta title","category":"Daily Use","steps":[]}
	]}`)
	if err := SeedGuideTopicsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}

	var alphaText string
	if err := db.QueryRowContext(ctx, `SELECT text FROM guide_topics WHERE key = 'Alpha'`).Scan(&alphaText); err != nil {
		t.Fatal(err)
	}
	if alphaText != `{"key":"Alpha","title":"Alpha title","category":"Daily Use","steps":[]}` {
		t.Fatalf("existing row was overwritten: %s", alphaText)
	}

	var betaCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM guide_topics WHERE key = 'Beta'`).Scan(&betaCount); err != nil {
		t.Fatal(err)
	}
	if betaCount != 1 {
		t.Fatalf("beta count = %d, want 1", betaCount)
	}
}

func TestReloadGuideTopicsFromFileReplacesAll(t *testing.T) {
	ctx := context.Background()
	db := openGuideTopicsTestDB(t, ctx)
	defer db.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "guide_topics.json")
	writeGuideTopicsSeedFile(t, path, `{"topics":[
		{"key":"Alpha","title":"Alpha title","category":"Daily Use","steps":[]},
		{"key":"Beta","title":"Beta title","category":"Daily Use","steps":[]}
	]}`)
	if err := SeedGuideTopicsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}

	writeGuideTopicsSeedFile(t, path, `{"topics":[
		{"key":"Alpha","title":"Updated alpha","category":"Daily Use","steps":[]},
		{"key":"Gamma","title":"Gamma title","category":"Daily Use","steps":[]}
	]}`)
	if err := ReloadGuideTopicsFromFile(ctx, db, path); err != nil {
		t.Fatal(err)
	}

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM guide_topics`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total rows = %d, want 2", total)
	}

	var betaCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM guide_topics WHERE key = 'Beta'`).Scan(&betaCount); err != nil {
		t.Fatal(err)
	}
	if betaCount != 0 {
		t.Fatalf("beta count = %d, want 0", betaCount)
	}

	var alphaText string
	if err := db.QueryRowContext(ctx, `SELECT text FROM guide_topics WHERE key = 'Alpha'`).Scan(&alphaText); err != nil {
		t.Fatal(err)
	}
	if alphaText != `{"key":"Alpha","title":"Updated alpha","category":"Daily Use","steps":[]}` {
		t.Fatalf("alpha text = %s", alphaText)
	}
}

func openGuideTopicsTestDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE guide_topics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL,
			text TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX uq_guide_topics_key ON guide_topics (key)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func writeGuideTopicsSeedFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

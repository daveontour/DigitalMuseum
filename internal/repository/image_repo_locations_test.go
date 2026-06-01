package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/model"
)

func TestResolveLocationCategorySources(t *testing.T) {
	allSources, allOther := resolveLocationCategorySources([]string{"all"})
	if !allOther {
		t.Fatalf("expected includeOther=true for all, got false")
	}
	if len(allSources) != len(knownLocationCategorySources) {
		t.Fatalf("all sources count = %d; want %d", len(allSources), len(knownLocationCategorySources))
	}

	sources, other := resolveLocationCategorySources([]string{"filesystem", "email", "other", "unknown"})
	if other != true {
		t.Fatalf("expected includeOther=true, got false")
	}
	want := map[string]struct{}{
		"filesystem": {}, "email_attachment": {}, "gmail_attachment": {},
	}
	if len(sources) != len(want) {
		t.Fatalf("sources = %v; want %d entries", sources, len(want))
	}
	for _, s := range sources {
		if _, ok := want[s]; !ok {
			t.Fatalf("unexpected source %q in %v", s, sources)
		}
	}
}

func TestGetRandomLocationsByCategories_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Skipf("sqlite3 unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE media_blobs (id INTEGER PRIMARY KEY, image_data BLOB, thumbnail_data BLOB);
		CREATE TABLE media_items (
			id INTEGER PRIMARY KEY,
			media_blob_id INTEGER NOT NULL,
			description TEXT, title TEXT, author TEXT, tags TEXT, categories TEXT, notes TEXT,
			available_for_task BOOLEAN NOT NULL DEFAULT 0,
			media_type TEXT, processed BOOLEAN NOT NULL DEFAULT 0,
			created_at TEXT, updated_at TEXT, embedding TEXT,
			year INTEGER, month INTEGER, day INTEGER,
			latitude REAL, longitude REAL, altitude REAL,
			rating INTEGER NOT NULL DEFAULT 5,
			has_gps BOOLEAN NOT NULL DEFAULT 0,
			google_maps_url TEXT, region TEXT,
			is_personal BOOLEAN NOT NULL DEFAULT 0,
			is_business BOOLEAN NOT NULL DEFAULT 0,
			is_social BOOLEAN NOT NULL DEFAULT 0,
			is_promotional BOOLEAN NOT NULL DEFAULT 0,
			is_spam BOOLEAN NOT NULL DEFAULT 0,
			is_important BOOLEAN NOT NULL DEFAULT 0,
			use_by_ai BOOLEAN DEFAULT 0,
			is_referenced BOOLEAN NOT NULL DEFAULT 0,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
		INSERT INTO media_blobs (id) VALUES (1), (2), (3);
		INSERT INTO media_items (
			id, media_blob_id, source, latitude, longitude, has_gps, user_id
		) VALUES
			(1, 1, 'filesystem', 51.5, -0.1, 1, 42),
			(2, 2, 'whatsapp', 40.7, -74.0, 1, 42),
			(3, 3, 'legacy_source', 48.8, 2.3, 0, 42);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))

	items, err := repo.GetRandomLocationsByCategories(ctx, []string{"filesystem"}, "", 10)
	if err != nil {
		t.Fatalf("GetRandomLocationsByCategories: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("filesystem count = %d; want 1", len(items))
	}
	if items[0].Source == nil || *items[0].Source != "filesystem" {
		t.Fatalf("unexpected item source: %+v", items[0].Source)
	}

	items, err = repo.GetRandomLocationsByCategories(ctx, []string{"other"}, "", 10)
	if err != nil {
		t.Fatalf("other category: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("other count = %d; want 1", len(items))
	}

	items, err = repo.GetRandomLocationsByCategories(ctx, []string{"filesystem", "whatsapp"}, "", 10)
	if err != nil {
		t.Fatalf("combined categories: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("combined count = %d; want 2", len(items))
	}
}

func TestGetRandomLocationsByCategories_RegionFilter_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Skipf("sqlite3 unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE media_blobs (id INTEGER PRIMARY KEY, image_data BLOB, thumbnail_data BLOB);
		CREATE TABLE media_items (
			id INTEGER PRIMARY KEY,
			media_blob_id INTEGER NOT NULL,
			description TEXT, title TEXT, author TEXT, tags TEXT, categories TEXT, notes TEXT,
			available_for_task BOOLEAN NOT NULL DEFAULT 0,
			media_type TEXT, processed BOOLEAN NOT NULL DEFAULT 0,
			created_at TEXT, updated_at TEXT, embedding TEXT,
			year INTEGER, month INTEGER, day INTEGER,
			latitude REAL, longitude REAL, altitude REAL,
			rating INTEGER NOT NULL DEFAULT 5,
			has_gps BOOLEAN NOT NULL DEFAULT 0,
			google_maps_url TEXT, region TEXT,
			is_personal BOOLEAN NOT NULL DEFAULT 0,
			is_business BOOLEAN NOT NULL DEFAULT 0,
			is_social BOOLEAN NOT NULL DEFAULT 0,
			is_promotional BOOLEAN NOT NULL DEFAULT 0,
			is_spam BOOLEAN NOT NULL DEFAULT 0,
			is_important BOOLEAN NOT NULL DEFAULT 0,
			use_by_ai BOOLEAN DEFAULT 0,
			is_referenced BOOLEAN NOT NULL DEFAULT 0,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
		INSERT INTO media_blobs (id) VALUES (1), (2);
		INSERT INTO media_items (
			id, media_blob_id, source, latitude, longitude, has_gps, region, user_id
		) VALUES
			(1, 1, 'filesystem', 51.5, -0.1, 1, 'eur', 42),
			(2, 2, 'filesystem', 40.7, -74.0, 1, 'usa', 42);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))

	items, err := repo.GetRandomLocationsByCategories(ctx, []string{"filesystem"}, "usa", 10)
	if err != nil {
		t.Fatalf("GetRandomLocationsByCategories region usa: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("usa count = %d; want 1", len(items))
	}
	if items[0].Region == nil || *items[0].Region != "usa" {
		t.Fatalf("unexpected region: %+v", items[0].Region)
	}
}

func TestGetRandomLocationsByCategories_VariesBetweenCalls_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Skipf("sqlite3 unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE media_blobs (id INTEGER PRIMARY KEY, image_data BLOB, thumbnail_data BLOB);
		CREATE TABLE media_items (
			id INTEGER PRIMARY KEY,
			media_blob_id INTEGER NOT NULL,
			description TEXT, title TEXT, author TEXT, tags TEXT, categories TEXT, notes TEXT,
			available_for_task BOOLEAN NOT NULL DEFAULT 0,
			media_type TEXT, processed BOOLEAN NOT NULL DEFAULT 0,
			created_at TEXT, updated_at TEXT, embedding TEXT,
			year INTEGER, month INTEGER, day INTEGER,
			latitude REAL, longitude REAL, altitude REAL,
			rating INTEGER NOT NULL DEFAULT 5,
			has_gps BOOLEAN NOT NULL DEFAULT 0,
			google_maps_url TEXT, region TEXT,
			is_personal BOOLEAN NOT NULL DEFAULT 0,
			is_business BOOLEAN NOT NULL DEFAULT 0,
			is_social BOOLEAN NOT NULL DEFAULT 0,
			is_promotional BOOLEAN NOT NULL DEFAULT 0,
			is_spam BOOLEAN NOT NULL DEFAULT 0,
			is_important BOOLEAN NOT NULL DEFAULT 0,
			use_by_ai BOOLEAN DEFAULT 0,
			is_referenced BOOLEAN NOT NULL DEFAULT 0,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	for i := 1; i <= 30; i++ {
		_, err = db.Exec(`
			INSERT INTO media_blobs (id) VALUES (?);
			INSERT INTO media_items (id, media_blob_id, source, latitude, longitude, has_gps)
			VALUES (?, ?, 'filesystem', ?, ?, 1);
		`, i, i, i, float64(i), float64(i)*0.1)
		if err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	repo := NewImageRepo(db)
	ctx := context.Background()

	for attempt := 0; attempt < 20; attempt++ {
		first, err := repo.GetRandomLocationsByCategories(ctx, []string{"filesystem"}, "", 10)
		if err != nil {
			t.Fatalf("GetRandomLocationsByCategories first: %v", err)
		}
		second, err := repo.GetRandomLocationsByCategories(ctx, []string{"filesystem"}, "", 10)
		if err != nil {
			t.Fatalf("GetRandomLocationsByCategories second: %v", err)
		}
		if len(first) != 10 || len(second) != 10 {
			t.Fatalf("sample size first=%d second=%d; want 10", len(first), len(second))
		}
		if !sameIDSet(idsFromItems(first), idsFromItems(second)) {
			return
		}
	}
	t.Fatalf("expected at least one differing random sample across repeated calls")
}

func idsFromItems(items []*model.MediaItem) map[int64]struct{} {
	ids := make(map[int64]struct{}, len(items))
	for _, item := range items {
		ids[item.ID] = struct{}{}
	}
	return ids
}

func sameIDSet(a, b map[int64]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}

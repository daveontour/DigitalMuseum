package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/daveontour/aimuseum/internal/appctx"
)

func TestListImageMetadataForJSONExport_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:metaexport?mode=memory&cache=shared")
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
			require_classification BOOLEAN NOT NULL DEFAULT 1,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
		INSERT INTO media_blobs (id) VALUES (1), (2), (3);
		INSERT INTO media_items (
			id, media_blob_id, media_type, source, source_reference,
			latitude, longitude, has_gps, google_maps_url, tags, user_id
		) VALUES
			(3, 1, 'image/jpeg', 'whatsapp', '144573', 25.14, 55.22, 1, 'https://maps.example/3', 'Playhouse Company', 42),
			(1, 2, 'image/jpeg', 'filesystem', 'ref-1', NULL, NULL, 0, NULL, 'alpha', 42),
			(2, 3, 'video/mp4', 'whatsapp', '999', NULL, NULL, 0, NULL, 'skip-me', 42);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	records, err := repo.ListImageMetadataForJSONExport(ctx)
	if err != nil {
		t.Fatalf("ListImageMetadataForJSONExport: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len = %d; want 2 image rows", len(records))
	}
	if records[0].ID != 1 || records[1].ID != 3 {
		t.Fatalf("order = [%d, %d]; want [1, 3]", records[0].ID, records[1].ID)
	}
	if records[1].HasGPS != true || records[1].Latitude == nil || *records[1].Latitude != 25.14 {
		t.Fatalf("record 3 gps: %+v", records[1])
	}
	if records[0].Tags == nil || *records[0].Tags != "alpha" {
		t.Fatalf("record 1 tags: %+v", records[0].Tags)
	}
}

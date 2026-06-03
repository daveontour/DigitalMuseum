package repository

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/georegion"
)

func TestBulkSetGPS_SQLite(t *testing.T) {
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
			require_classification BOOLEAN NOT NULL DEFAULT 1,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
		INSERT INTO media_blobs (id) VALUES (1), (2), (3), (4);
		INSERT INTO media_items (
			id, media_blob_id, source, latitude, longitude, has_gps, user_id
		) VALUES
			(1, 1, 'filesystem', NULL, NULL, 0, 42),
			(2, 2, 'filesystem', 51.5, -0.1, 1, 42),
			(3, 3, 'filesystem', NULL, NULL, 0, 99),
			(4, 4, 'filesystem', NULL, NULL, 0, 42);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	regionsPath := filepath.Join("..", "georegion", "testdata", "regions_test.json")
	if err := georegion.Load(regionsPath); err != nil {
		t.Fatalf("georegion.Load: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))

	updated, skipped, updates, err := repo.BulkSetGPS(ctx, []int64{1, 2, 3, 999}, -33.87, 151.21)
	if err != nil {
		t.Fatalf("BulkSetGPS: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d; want 1", updated)
	}
	if len(skipped) != 3 {
		t.Fatalf("skipped len = %d; want 3 (gps, wrong user, missing)", len(skipped))
	}
	if len(updates) != 1 {
		t.Fatalf("updates len = %d; want 1", len(updates))
	}

	var lat, lng float64
	var hasGPS bool
	var mapsURL, region sql.NullString
	err = db.QueryRow(`SELECT latitude, longitude, has_gps, google_maps_url, region FROM media_items WHERE id = 1`).
		Scan(&lat, &lng, &hasGPS, &mapsURL, &region)
	if err != nil {
		t.Fatalf("query row 1: %v", err)
	}
	if !hasGPS || lat != -33.87 || lng != 151.21 {
		t.Fatalf("row 1 gps: has_gps=%v lat=%v lng=%v", hasGPS, lat, lng)
	}
	if !mapsURL.Valid || mapsURL.String == "" {
		t.Fatalf("row 1 missing google_maps_url")
	}
	if !region.Valid || region.String == "" {
		t.Fatalf("row 1 missing region")
	}

	// Wrong user row unchanged
	var lat3 sql.NullFloat64
	err = db.QueryRow(`SELECT latitude FROM media_items WHERE id = 3`).Scan(&lat3)
	if err != nil {
		t.Fatalf("query row 3: %v", err)
	}
	if lat3.Valid {
		t.Fatalf("row 3 should have NULL latitude")
	}
}

func TestBulkSetGPS_SpreadOnCircle_SQLite(t *testing.T) {
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
			require_classification BOOLEAN NOT NULL DEFAULT 1,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
		INSERT INTO media_blobs (id) VALUES (10), (11), (12);
		INSERT INTO media_items (id, media_blob_id, source, latitude, longitude, has_gps, user_id) VALUES
			(10, 10, 'filesystem', NULL, NULL, 0, 42),
			(11, 11, 'filesystem', NULL, NULL, 0, 42),
			(12, 12, 'filesystem', NULL, NULL, 0, 42);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	regionsPath := filepath.Join("..", "georegion", "testdata", "regions_test.json")
	if err := georegion.Load(regionsPath); err != nil {
		t.Fatalf("georegion.Load: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	centerLat, centerLng := -33.87, 151.21

	updated, _, updates, err := repo.BulkSetGPS(ctx, []int64{10, 11, 12}, centerLat, centerLng)
	if err != nil {
		t.Fatalf("BulkSetGPS: %v", err)
	}
	if updated != 3 || len(updates) != 3 {
		t.Fatalf("updated=%d updates=%d; want 3 each", updated, len(updates))
	}

	coords := make(map[int64][2]float64)
	for _, u := range updates {
		coords[u.ID] = [2]float64{u.Latitude, u.Longitude}
	}
	for id, c := range coords {
		dist := haversineMeters(centerLat, centerLng, c[0], c[1])
		if dist < 95 || dist > 105 {
			t.Fatalf("id %d distance from center %.1fm; want ~100m", id, dist)
		}
	}
	// All three should be distinct
	if coords[10] == coords[11] || coords[10] == coords[12] || coords[11] == coords[12] {
		t.Fatalf("expected distinct coordinates: %+v", coords)
	}
}

func TestClearGPSOnMediaItem_SQLite(t *testing.T) {
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
			require_classification BOOLEAN NOT NULL DEFAULT 1,
			source TEXT, source_reference TEXT,
			user_id INTEGER
		);
		INSERT INTO media_blobs (id) VALUES (1), (2);
		INSERT INTO media_items (
			id, media_blob_id, source, latitude, longitude, has_gps, google_maps_url, region, user_id
		) VALUES
			(1, 1, 'filesystem', 25.142245, 55.222816, 1, 'https://maps.example/1', 'AE-DU', 42),
			(2, 2, 'filesystem', 51.5, -0.1, 1, 'https://maps.example/2', 'GB-LON', 99);
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))

	ok, err := repo.ClearGPSOnMediaItem(ctx, 1)
	if err != nil {
		t.Fatalf("ClearGPSOnMediaItem: %v", err)
	}
	if !ok {
		t.Fatal("expected row updated")
	}

	var lat, lng sql.NullFloat64
	var hasGPS bool
	var mapsURL, region sql.NullString
	err = db.QueryRow(`SELECT latitude, longitude, has_gps, google_maps_url, region FROM media_items WHERE id = 1`).
		Scan(&lat, &lng, &hasGPS, &mapsURL, &region)
	if err != nil {
		t.Fatalf("query row 1: %v", err)
	}
	if lat.Valid || lng.Valid || hasGPS || mapsURL.Valid || region.Valid {
		t.Fatalf("row 1 gps not cleared: lat=%v lng=%v has_gps=%v maps=%v region=%v", lat, lng, hasGPS, mapsURL, region)
	}

	okWrongUser, err := repo.ClearGPSOnMediaItem(ctx, 2)
	if err != nil {
		t.Fatalf("ClearGPSOnMediaItem wrong user: %v", err)
	}
	if okWrongUser {
		t.Fatal("expected no update for wrong user_id")
	}
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthR = 6371000.0
	rad := func(d float64) float64 { return d * 3.141592653589793 / 180 }
	dLat := rad(lat2 - lat1)
	dLng := rad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthR * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/georegion"
)

func setupDuplicateGPSTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:dupgps?mode=memory&cache=shared")
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
	`)
	if err != nil {
		t.Fatalf("schema setup: %v", err)
	}
	return db
}

func loadTestRegions(t *testing.T) {
	t.Helper()
	regionsPath := filepath.Join("..", "georegion", "testdata", "regions_test.json")
	if err := georegion.Load(regionsPath); err != nil {
		t.Fatalf("georegion.Load: %v", err)
	}
}

func TestGPSDuplicateSpreadRadiusForCount(t *testing.T) {
	if GPSDuplicateSpreadRadiusForCount(2) != GPSSpreadRadiusStandardM {
		t.Fatalf("count 2: got %v", GPSDuplicateSpreadRadiusForCount(2))
	}
	if GPSDuplicateSpreadRadiusForCount(20) != GPSSpreadRadiusStandardM {
		t.Fatalf("count 20: got %v", GPSDuplicateSpreadRadiusForCount(20))
	}
	if GPSDuplicateSpreadRadiusForCount(21) != GPSSpreadRadiusLargeM {
		t.Fatalf("count 21: got %v", GPSDuplicateSpreadRadiusForCount(21))
	}
}

func TestListDuplicateImageGPSGroups_SQLite(t *testing.T) {
	db := setupDuplicateGPSTestDB(t)
	_, err := db.Exec(`
		INSERT INTO media_blobs (id) VALUES (1), (2), (3), (4), (5);
		INSERT INTO media_items (id, media_blob_id, media_type, latitude, longitude, has_gps, user_id) VALUES
			(1, 1, 'image/jpeg', -33.87, 151.21, 1, 42),
			(2, 2, 'image/jpeg', -33.87, 151.21, 1, 42),
			(3, 3, 'image/jpeg', -33.87, 151.21, 1, 42),
			(4, 4, 'image/jpeg', 40.0, -74.0, 1, 42),
			(5, 5, 'video/mp4', -33.87, 151.21, 1, 42);
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	groups, err := repo.ListDuplicateImageGPSGroups(ctx)
	if err != nil {
		t.Fatalf("ListDuplicateImageGPSGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d; want 1", len(groups))
	}
	if len(groups[0].IDs) != 3 {
		t.Fatalf("group ids len = %d; want 3 (video row excluded)", len(groups[0].IDs))
	}
}

func TestSpreadImageGPSOnCircle_StandardRadius_SQLite(t *testing.T) {
	db := setupDuplicateGPSTestDB(t)
	_, err := db.Exec(`
		INSERT INTO media_blobs (id) VALUES (10), (11), (12);
		INSERT INTO media_items (id, media_blob_id, media_type, latitude, longitude, has_gps, user_id) VALUES
			(10, 10, 'image/jpeg', -33.87, 151.21, 1, 42),
			(11, 11, 'image/jpeg', -33.87, 151.21, 1, 42),
			(12, 12, 'image/jpeg', -33.87, 151.21, 1, 42);
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	loadTestRegions(t)

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	centerLat, centerLng := -33.87, 151.21
	ids := []int64{10, 11, 12}

	updated, err := repo.SpreadImageGPSOnCircle(ctx, ids, centerLat, centerLng, GPSSpreadRadiusStandardM)
	if err != nil {
		t.Fatalf("SpreadImageGPSOnCircle: %v", err)
	}
	if updated != 3 {
		t.Fatalf("updated = %d; want 3", updated)
	}

	coords := make(map[int64][2]float64)
	for _, id := range ids {
		var lat, lng float64
		if err := db.QueryRow(`SELECT latitude, longitude FROM media_items WHERE id = ?`, id).Scan(&lat, &lng); err != nil {
			t.Fatalf("query id %d: %v", id, err)
		}
		coords[id] = [2]float64{lat, lng}
		dist := haversineMeters(centerLat, centerLng, lat, lng)
		if dist < 95 || dist > 105 {
			t.Fatalf("id %d distance %.1fm; want ~100m", id, dist)
		}
	}
	if coords[10] == coords[11] || coords[10] == coords[12] || coords[11] == coords[12] {
		t.Fatalf("expected distinct coordinates: %+v", coords)
	}
}

func TestSpreadImageGPSOnCircle_LargeRadius_SQLite(t *testing.T) {
	db := setupDuplicateGPSTestDB(t)
	_, err := db.Exec(`
		INSERT INTO media_blobs (id) VALUES (20), (21);
		INSERT INTO media_items (id, media_blob_id, media_type, latitude, longitude, has_gps, user_id) VALUES
			(20, 20, 'image/jpeg', -33.87, 151.21, 1, 42),
			(21, 21, 'image/jpeg', -33.87, 151.21, 1, 42);
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	loadTestRegions(t)

	repo := NewImageRepo(db)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	centerLat, centerLng := -33.87, 151.21

	updated, err := repo.SpreadImageGPSOnCircle(ctx, []int64{20, 21}, centerLat, centerLng, GPSSpreadRadiusLargeM)
	if err != nil {
		t.Fatalf("SpreadImageGPSOnCircle: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d; want 2", updated)
	}

	for _, id := range []int64{20, 21} {
		var lat, lng float64
		if err := db.QueryRow(`SELECT latitude, longitude FROM media_items WHERE id = ?`, id).Scan(&lat, &lng); err != nil {
			t.Fatalf("query id %d: %v", id, err)
		}
		dist := haversineMeters(centerLat, centerLng, lat, lng)
		if dist < 145 || dist > 155 {
			t.Fatalf("id %d distance %.1fm; want ~150m", id, dist)
		}
	}
}

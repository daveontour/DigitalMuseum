package georegion

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestRegionFromLatLng(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		lng  float64
		want string
	}{
		{name: "australia", lat: -33.8688, lng: 151.2093, want: "aus"},
		{name: "dubai", lat: 25.2048, lng: 55.2708, want: "dxb"},
		{name: "ireland", lat: 53.3498, lng: -6.2603, want: "ireland"},
		{name: "uk", lat: 51.5074, lng: -0.1278, want: "uk"},
		{name: "europe", lat: 48.8566, lng: 2.3522, want: "eur"},
		{name: "usa", lat: 40.7128, lng: -74.0060, want: "usa"},
		{name: "africa", lat: -1.2921, lng: 36.8219, want: "af"},
		{name: "middle_east", lat: 35.6892, lng: 51.3890, want: "me"},
		{name: "asia", lat: 35.6762, lng: 139.6503, want: "asia"},
		{name: "central_america", lat: 19.4326, lng: -99.1332, want: "central_america"},
		{name: "caribbean", lat: 18.2208, lng: -66.5901, want: "carribean"},
		{name: "new_zealand", lat: -36.8485, lng: 174.7633, want: "nz"},
		{name: "south_america", lat: -23.5505, lng: -46.6333, want: "south_america"},
		{name: "other", lat: -90.0, lng: 0.0, want: "oth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegionFromLatLng(tt.lat, tt.lng)
			if got != tt.want {
				t.Fatalf("RegionFromLatLng(%v, %v) = %q, want %q", tt.lat, tt.lng, got, tt.want)
			}
		})
	}
}

func TestUpdateMediaItemRegions(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE media_items (
			id INTEGER PRIMARY KEY,
			latitude REAL,
			longitude REAL,
			region TEXT
		);
		INSERT INTO media_items (id, latitude, longitude, region) VALUES
			(1, -33.8688, 151.2093, NULL),
			(2, 40.7128, -74.0060, 'old'),
			(3, NULL, NULL, NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateMediaItemRegions(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	var region1, region2, region3 sql.NullString
	err = db.QueryRow(`SELECT region FROM media_items WHERE id = 1`).Scan(&region1)
	if err != nil {
		t.Fatal(err)
	}
	if !region1.Valid || region1.String != "aus" {
		t.Fatalf("id=1 region = %v, want aus", region1)
	}

	err = db.QueryRow(`SELECT region FROM media_items WHERE id = 2`).Scan(&region2)
	if err != nil {
		t.Fatal(err)
	}
	if !region2.Valid || region2.String != "usa" {
		t.Fatalf("id=2 region = %v, want usa", region2)
	}

	err = db.QueryRow(`SELECT region FROM media_items WHERE id = 3`).Scan(&region3)
	if err != nil {
		t.Fatal(err)
	}
	if region3.Valid {
		t.Fatalf("id=3 region = %v, want NULL", region3)
	}
}

func TestUpdateLocationRegions(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE locations (
			id INTEGER PRIMARY KEY,
			latitude REAL,
			longitude REAL,
			region TEXT
		);
		INSERT INTO locations (id, latitude, longitude, region) VALUES
			(1, 48.8566, 2.3522, NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := UpdateLocationRegions(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	var region sql.NullString
	err = db.QueryRow(`SELECT region FROM locations WHERE id = 1`).Scan(&region)
	if err != nil {
		t.Fatal(err)
	}
	if !region.Valid || region.String != "eur" {
		t.Fatalf("id=1 region = %v, want eur", region)
	}
}

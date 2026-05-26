package georegion

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestRegistryFromDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

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

	path := filepath.Join("testdata", "regions_test.json")
	if err := seedTestRegionsFromFile(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	if err := ReloadFromDB(ctx, db); err != nil {
		t.Fatal(err)
	}
}

func seedTestRegionsFromFile(ctx context.Context, db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if err := ValidateConfig(&cfg); err != nil {
		return err
	}
	if err := insertTestRegionKey(ctx, db, KeyDefaultRegion, 0, cfg.DefaultRegion); err != nil {
		return err
	}
	if err := insertTestRegionKey(ctx, db, KeyDefaultLabel, 1, cfg.DefaultLabel); err != nil {
		return err
	}
	for i, r := range cfg.Regions {
		if err := insertTestRegionKey(ctx, db, r.Code, i+2, r); err != nil {
			return err
		}
	}
	return nil
}

func insertTestRegionKey(ctx context.Context, db *sql.DB, key string, sortOrder int, value any) error {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM regions WHERE key = ?`, key).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	text, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO regions (key, sort_order, text) VALUES (?, ?, ?)`, key, sortOrder, string(text))
	return err
}

func TestMain(m *testing.M) {
	// File-based validation tests do not need the default registry.
	os.Exit(m.Run())
}

func TestReloadFromDB(t *testing.T) {
	setupTestRegistryFromDB(t)
	reg := Default()
	if reg.Label("aus") != "Australia" {
		t.Fatalf("Label(aus) = %q, want Australia", reg.Label("aus"))
	}
	if reg.RegionFromLatLng(-33.8688, 151.2093) != "aus" {
		t.Fatalf("unexpected australia lookup")
	}
}

func TestLoadValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("duplicate code", func(t *testing.T) {
		p := write("dup.json", `{"default_region":"oth","default_label":"Other","regions":[
			{"code":"aus","label":"A","bbox":[1,2,3,4]},
			{"code":"aus","label":"B","bbox":[5,6,7,8]}
		]}`)
		if _, err := loadRegistry(p); err == nil {
			t.Fatal("expected duplicate code error")
		}
	})

	t.Run("bad bbox length", func(t *testing.T) {
		p := write("badbbox.json", `{"default_region":"oth","default_label":"Other","regions":[
			{"code":"aus","label":"A","bbox":[1,2,3]}
		]}`)
		if _, err := loadRegistry(p); err == nil {
			t.Fatal("expected bbox length error")
		}
	})

	t.Run("min greater than max", func(t *testing.T) {
		p := write("minmax.json", `{"default_region":"oth","default_label":"Other","regions":[
			{"code":"aus","label":"A","bbox":[10,20,5,25]}
		]}`)
		if _, err := loadRegistry(p); err == nil {
			t.Fatal("expected min_lon > max_lon error")
		}
	})
}

func TestLabel(t *testing.T) {
	setupTestRegistryFromDB(t)
	reg := Default()
	if got := reg.Label("aus"); got != "Australia" {
		t.Fatalf("Label(aus) = %q, want Australia", got)
	}
	if got := reg.Label("oth"); got != "Other" {
		t.Fatalf("Label(oth) = %q, want Other", got)
	}
	if got := reg.Label(""); got != "Unknown" {
		t.Fatalf("Label(empty) = %q, want Unknown", got)
	}
	if got := reg.Label("stale_code"); got != "stale_code" {
		t.Fatalf("Label(unknown) = %q, want stale_code", got)
	}
}

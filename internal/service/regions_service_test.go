package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func setupRegionsServiceTest(t *testing.T) (*RegionsService, context.Context) {
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

	path := filepath.Join("..", "georegion", "testdata", "regions_test.json")
	if err := database.SeedRegionsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	if err := georegion.ReloadFromDB(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRegionsRepo(db)
	return NewRegionsService(repo, db), ctx
}

func TestRegionsServiceExportConfig(t *testing.T) {
	svc, ctx := setupRegionsServiceTest(t)
	cfg, err := svc.ExportConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRegion != "oth" {
		t.Fatalf("DefaultRegion = %q, want oth", cfg.DefaultRegion)
	}
	if len(cfg.Regions) == 0 {
		t.Fatal("expected regions in export")
	}
}

func TestRegionsServiceImportPreviewConflicts(t *testing.T) {
	svc, ctx := setupRegionsServiceTest(t)
	upload := georegion.Config{
		DefaultRegion: "oth",
		DefaultLabel:  "Other",
		Regions: []georegion.RegionDefinition{
			{Code: "aus", Label: "Uploaded Australia", BBox: []float64{1, 2, 3, 4}},
			{Code: "brand_new", Label: "Brand New", BBox: []float64{10, 11, 12, 13}},
		},
	}
	data, err := json.Marshal(upload)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := svc.ImportPreview(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.New) != 1 || preview.New[0].Code != "brand_new" {
		t.Fatalf("preview new = %+v, want brand_new", preview.New)
	}
	if len(preview.Conflicts) != 1 || preview.Conflicts[0].Key != "aus" {
		t.Fatalf("preview conflicts = %+v, want aus conflict", preview.Conflicts)
	}
}

func TestRegionsServiceImportApplyReplace(t *testing.T) {
	svc, ctx := setupRegionsServiceTest(t)
	upload := georegion.Config{
		DefaultRegion: "oth",
		DefaultLabel:  "Other",
		Regions: []georegion.RegionDefinition{
			{Code: "aus", Label: "Replaced Australia", BBox: []float64{1, 2, 3, 4}},
		},
	}
	data, err := json.Marshal(upload)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ImportApply(ctx, data, map[string]string{"aus": "replace"}); err != nil {
		t.Fatal(err)
	}
	if got := georegion.Label("aus"); got != "Replaced Australia" {
		t.Fatalf("Label(aus) after import = %q", got)
	}
}

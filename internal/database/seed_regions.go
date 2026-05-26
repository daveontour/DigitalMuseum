package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/daveontour/aimuseum/internal/georegion"
)

// SeedRegionsFromFileIfMissing inserts region rows from regions.json when keys are absent.
// Existing rows are never updated.
func SeedRegionsFromFileIfMissing(ctx context.Context, db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read regions seed file %s: %w", path, err)
	}
	var cfg georegion.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse regions seed file %s: %w", path, err)
	}
	if err := georegion.ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("validate regions seed file %s: %w", path, err)
	}

	if err := insertRegionKeyIfMissing(ctx, db, georegion.KeyDefaultRegion, 0, cfg.DefaultRegion); err != nil {
		return err
	}
	if err := insertRegionKeyIfMissing(ctx, db, georegion.KeyDefaultLabel, 1, cfg.DefaultLabel); err != nil {
		return err
	}
	for i, r := range cfg.Regions {
		if err := insertRegionKeyIfMissing(ctx, db, r.Code, i+2, r); err != nil {
			return err
		}
	}
	return nil
}

func insertRegionKeyIfMissing(ctx context.Context, db *sql.DB, key string, sortOrder int, value any) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM regions WHERE key = ?`, key).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check region key %q: %w", key, err)
	}
	if exists > 0 {
		return nil
	}
	text, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal region key %q: %w", key, err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO regions (key, sort_order, text) VALUES (?, ?, ?)`,
		key, sortOrder, string(text),
	)
	if err != nil {
		return fmt.Errorf("insert region key %q: %w", key, err)
	}
	return nil
}

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

// SeedAIModelsFromFileIfMissing inserts ai_models rows from the seed JSON file
// when their key is not already present (insert-if-missing; tolerates a
// missing file so a fresh install without the seed file still boots). It also
// ensures the special "localai" row always exists (see insertLocalAIModelIfMissing),
// independent of the seed file, so Local AI can always be enabled/disabled and
// reordered on the AI Models tab like any hosted model.
func SeedAIModelsFromFileIfMissing(ctx context.Context, db *sql.DB, path string) error {
	items, err := readAIModelsSeedItems(path)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := insertAIModelKeyIfMissing(ctx, db, item); err != nil {
			return err
		}
	}
	return insertLocalAIModelIfMissing(ctx, db)
}

// insertLocalAIModelIfMissing seeds the "localai" ai_models row once, if it doesn't already
// exist. It's not routed through OpenRouter (model_slug is left blank and unused — see
// ChatService.effectiveProviderByKey), and defaults to sort_order -1 so it appears first,
// ahead of any hosted models, until an admin reorders it.
func insertLocalAIModelIfMissing(ctx context.Context, db *sql.DB) error {
	return insertAIModelKeyIfMissing(ctx, db, aiModelSeedRow{
		Key:         "localai",
		DisplayName: "Local AI",
		ModelSlug:   "",
		SortOrder:   -1,
	})
}

type aiModelSeedRow struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	ModelSlug   string `json:"model_slug"`
	Enabled     *bool  `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

func readAIModelsSeedItems(path string) ([]aiModelSeedRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read ai models seed file %s: %w", path, err)
	}

	var root struct {
		Models []aiModelSeedRow `json:"models"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse ai models seed file %s: %w", path, err)
	}

	out := make([]aiModelSeedRow, 0, len(root.Models))
	for _, item := range root.Models {
		if item.Key == "" || item.ModelSlug == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func insertAIModelKeyIfMissing(ctx context.Context, db *sql.DB, item aiModelSeedRow) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM ai_models WHERE key = ?`, item.Key).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check ai model key %q: %w", item.Key, err)
	}
	if exists > 0 {
		return nil
	}
	enabled := true
	if item.Enabled != nil {
		enabled = *item.Enabled
	}
	displayName := item.DisplayName
	if displayName == "" {
		displayName = item.Key
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO ai_models (key, display_name, model_slug, enabled, sort_order) VALUES (?, ?, ?, ?, ?)`,
		item.Key, displayName, item.ModelSlug, enabled, item.SortOrder)
	if err != nil {
		return fmt.Errorf("insert ai model key %q: %w", item.Key, err)
	}
	return nil
}

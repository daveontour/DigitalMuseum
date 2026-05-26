package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/daveontour/aimuseum/internal/suggestions"
)

type suggestionsSeedFile struct {
	Categories []suggestionsSeedCategory `json:"categories"`
}

type suggestionsSeedCategory struct {
	Category    string                   `json:"category"`
	Suggestions []map[string]any         `json:"suggestions"`
}

// SeedSuggestionsFromFileIfMissing inserts suggestion rows from suggestions.json when keys are absent.
func SeedSuggestionsFromFileIfMissing(ctx context.Context, db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read suggestions seed file %s: %w", path, err)
	}
	var file suggestionsSeedFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse suggestions seed file %s: %w", path, err)
	}
	for _, cat := range file.Categories {
		category := cat.Category
		for _, item := range cat.Suggestions {
			title, _ := item["title"].(string)
			key, err := suggestions.BuildKey(category, title)
			if err != nil {
				continue
			}
			text, err := json.Marshal(item)
			if err != nil {
				return fmt.Errorf("marshal suggestion %q: %w", key, err)
			}
			if err := insertSuggestionKeyIfMissing(ctx, db, key, string(text)); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertSuggestionKeyIfMissing(ctx context.Context, db *sql.DB, key, text string) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM suggestions WHERE key = ?`, key).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check suggestion key %q: %w", key, err)
	}
	if exists > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `INSERT INTO suggestions (key, text) VALUES (?, ?)`, key, text)
	if err != nil {
		return fmt.Errorf("insert suggestion key %q: %w", key, err)
	}
	return nil
}

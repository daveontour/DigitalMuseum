package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// SeedGuideTopicsFromFileIfMissing inserts guide_topics rows from guide_topics.json when keys are absent.
func SeedGuideTopicsFromFileIfMissing(ctx context.Context, db *sql.DB, path string) error {
	return seedGuideTopicsFromFile(ctx, db, path, false)
}

// ReloadGuideTopicsFromFile deletes all guide_topics rows and inserts every topic from the seed file.
func ReloadGuideTopicsFromFile(ctx context.Context, db *sql.DB, path string) error {
	return seedGuideTopicsFromFile(ctx, db, path, true)
}

func seedGuideTopicsFromFile(ctx context.Context, db *sql.DB, path string, replaceAll bool) error {
	items, err := readGuideTopicsSeedItems(path, replaceAll)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	if replaceAll {
		res, err := db.ExecContext(ctx, `DELETE FROM guide_topics`)
		if err != nil {
			return fmt.Errorf("clear guide_topics: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil {
			slog.Info("guide topics cleared for reload from file", "deleted", n, "path", path)
		}
	}

	inserted := 0
	for _, item := range items {
		if replaceAll {
			if err := insertGuideTopicRow(ctx, db, item.key, item.text); err != nil {
				return err
			}
			inserted++
			continue
		}
		if err := insertGuideTopicKeyIfMissing(ctx, db, item.key, item.text); err != nil {
			return err
		}
	}

	if replaceAll {
		slog.Info("guide topics reloaded from file", "inserted", inserted, "path", path)
	}
	return nil
}

type guideTopicSeedRow struct {
	key  string
	text string
}

func readGuideTopicsSeedItems(path string, requireFile bool) ([]guideTopicSeedRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if requireFile {
				return nil, fmt.Errorf("guide topics seed file not found: %s", path)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("read guide topics seed file %s: %w", path, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse guide topics seed file %s: %w", path, err)
	}

	topicsRaw, ok := raw["topics"]
	if !ok {
		if requireFile {
			return nil, fmt.Errorf("guide topics seed file %s: missing topics array", path)
		}
		return nil, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(topicsRaw, &items); err != nil {
		return nil, fmt.Errorf("parse guide topics array in %s: %w", path, err)
	}

	out := make([]guideTopicSeedRow, 0, len(items))
	for _, itemRaw := range items {
		var item struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			continue
		}
		if item.Key == "" {
			continue
		}
		out = append(out, guideTopicSeedRow{key: item.Key, text: string(itemRaw)})
	}
	if requireFile && len(out) == 0 {
		return nil, fmt.Errorf("guide topics seed file %s: no valid topics found", path)
	}
	return out, nil
}

func insertGuideTopicRow(ctx context.Context, db *sql.DB, key, text string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO guide_topics (key, text) VALUES (?, ?)`, key, text)
	if err != nil {
		return fmt.Errorf("insert guide topic key %q: %w", key, err)
	}
	return nil
}

func insertGuideTopicKeyIfMissing(ctx context.Context, db *sql.DB, key, text string) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM guide_topics WHERE key = ?`, key).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check guide topic key %q: %w", key, err)
	}
	if exists > 0 {
		return nil
	}
	return insertGuideTopicRow(ctx, db, key, text)
}

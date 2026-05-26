package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

type guideTopicsSeedFile struct {
	Topics []guideTopicSeedItem `json:"topics"`
}

type guideTopicSeedItem struct {
	Key string `json:"key"`
}

// SeedGuideTopicsFromFileIfMissing inserts guide_topics rows from guide_topics.json when keys are absent.
func SeedGuideTopicsFromFileIfMissing(ctx context.Context, db *sql.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read guide topics seed file %s: %w", path, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse guide topics seed file %s: %w", path, err)
	}

	topicsRaw, ok := raw["topics"]
	if !ok {
		return nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(topicsRaw, &items); err != nil {
		return fmt.Errorf("parse guide topics array in %s: %w", path, err)
	}

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
		if err := insertGuideTopicKeyIfMissing(ctx, db, item.Key, string(itemRaw)); err != nil {
			return err
		}
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
	_, err = db.ExecContext(ctx, `INSERT INTO guide_topics (key, text) VALUES (?, ?)`, key, text)
	if err != nil {
		return fmt.Errorf("insert guide topic key %q: %w", key, err)
	}
	return nil
}

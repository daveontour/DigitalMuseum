package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/suggestions"
	_ "github.com/mattn/go-sqlite3"
)

func setupSuggestionsServiceTest(t *testing.T) (*SuggestionsService, *repository.SuggestionsRepo, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE suggestions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL,
			text TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX uq_suggestions_key ON suggestions (key)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join("..", "..", "static", "data", "suggestions.json")
	if err := database.SeedSuggestionsFromFileIfMissing(ctx, db, path); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewSuggestionsRepo(db)
	return NewSuggestionsService(repo), repo, ctx
}

func TestSuggestionsServiceBuildCategoriesDocument(t *testing.T) {
	svc, _, ctx := setupSuggestionsServiceTest(t)
	doc, err := svc.BuildCategoriesDocument(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cats, ok := doc["categories"].([]any)
	if !ok || len(cats) == 0 {
		t.Fatal("expected categories in document")
	}
}

func TestSuggestionsServiceImportPreviewConflicts(t *testing.T) {
	svc, _, ctx := setupSuggestionsServiceTest(t)
	upload := map[string]any{
		"categories": []any{
			map[string]any{
				"category": "Getting started",
				"suggestions": []any{
					map[string]any{"title": "Early life overview", "prompt": "Uploaded prompt"},
					map[string]any{"title": "Brand new item", "prompt": "New prompt"},
				},
			},
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
	if len(preview.New) != 1 || preview.New[0].Suggestion["title"] != "Brand new item" {
		t.Fatalf("preview new = %+v", preview.New)
	}
	key, _ := suggestions.BuildKey("Getting started", "Early life overview")
	if len(preview.Conflicts) != 1 || preview.Conflicts[0].Key != key {
		t.Fatalf("preview conflicts = %+v", preview.Conflicts)
	}
}

func TestSuggestionsServiceImportApplyReplace(t *testing.T) {
	svc, repo, ctx := setupSuggestionsServiceTest(t)
	key, _ := suggestions.BuildKey("Getting started", "Early life overview")
	upload := map[string]any{
		"categories": []any{
			map[string]any{
				"category": "Getting started",
				"suggestions": []any{
					map[string]any{"title": "Early life overview", "prompt": "Replaced prompt"},
				},
			},
		},
	}
	data, err := json.Marshal(upload)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ImportApply(ctx, data, map[string]string{key: "replace"}); err != nil {
		t.Fatal(err)
	}
	row, err := repo.GetByKey(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(row.Text), &item); err != nil {
		t.Fatal(err)
	}
	if item["prompt"] != "Replaced prompt" {
		t.Fatalf("prompt after replace = %v", item["prompt"])
	}
}

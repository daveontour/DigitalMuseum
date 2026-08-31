package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/daveontour/aimuseum/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

// newTestChatServiceWithModels builds a ChatService backed by a real, in-memory
// AIModelsService seeded with the given keys (in that sort_order), for testing the
// dynamic-model-aware hosted-provider-order logic without a full server wiring.
func newTestChatServiceWithModels(t *testing.T, keys ...string) (*ChatService, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE ai_models (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			key          TEXT NOT NULL,
			display_name TEXT NOT NULL,
			model_slug   TEXT NOT NULL,
			enabled      INTEGER NOT NULL DEFAULT 1,
			sort_order   INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX uq_ai_models_key ON ai_models (key)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for i, key := range keys {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO ai_models (key, display_name, model_slug, enabled, sort_order) VALUES (?, ?, ?, 1, ?)`,
			key, key, key+"/model", i,
		); err != nil {
			t.Fatal(err)
		}
	}

	repo := repository.NewAIModelsRepo(db)
	svc := NewAIModelsService(repo)
	return &ChatService{aiModelsSvc: svc}, ctx
}

func TestDefaultHostedLLMProviderOrder(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "gemini", "claude", "deepseek", "openai")
	got := s.defaultHostedLLMProviderOrder(ctx)
	want := []string{"gemini", "claude", "deepseek", "openai"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestHostedProviderTryOrder(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "gemini", "claude", "deepseek", "openai")
	tests := []struct {
		lastManual string
		want       []string
	}{
		{"claude", []string{"claude", "gemini", "deepseek", "openai"}},
		{"", []string{"gemini", "claude", "deepseek", "openai"}},
		{"invalid", []string{"gemini", "claude", "deepseek", "openai"}},
	}
	for _, tc := range tests {
		got := s.HostedProviderTryOrder(ctx, tc.lastManual)
		if len(got) != len(tc.want) {
			t.Fatalf("lastManual=%q: got %v want %v", tc.lastManual, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("lastManual=%q: got %v want %v", tc.lastManual, got, tc.want)
			}
		}
	}
}

func TestFailoverProviderTryOrder(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "gemini", "claude", "deepseek", "openai")
	got := s.FailoverProviderTryOrder(ctx, "gemini")
	want := []string{"claude", "deepseek", "openai"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestParseHostedLLMProviderOrderJSON(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "gemini", "claude", "deepseek", "openai")
	cfg := s.parseHostedLLMProviderOrderJSON(ctx, `{"classifier_provider":"gemini","failover_enabled":true}`)
	if cfg.ClassifierProvider != "gemini" {
		t.Fatalf("classifier=%q want gemini", cfg.ClassifierProvider)
	}
	if !cfg.FailoverEnabled {
		t.Fatalf("failover_enabled default true, got false")
	}
	cfgDisabled := s.parseHostedLLMProviderOrderJSON(ctx, `{"failover_enabled":false}`)
	if cfgDisabled.FailoverEnabled {
		t.Fatalf("failover_enabled=false not parsed")
	}
	cfgDefault := s.parseHostedLLMProviderOrderJSON(ctx, "")
	if cfgDefault.ClassifierProvider != DefaultClassifierProvider {
		t.Fatalf("default classifier=%q", cfgDefault.ClassifierProvider)
	}
	if !cfgDefault.FailoverEnabled {
		t.Fatalf("default failover_enabled should be true")
	}
}

// modelRow is one row for newTestChatServiceWithModelRows, allowing explicit control over
// enabled/sort_order (unlike newTestChatServiceWithModels, which always enables every key
// in argument order) — needed to test disabling/reordering the "localai" row.
type modelRow struct {
	key       string
	enabled   bool
	sortOrder int
}

func newTestChatServiceWithModelRows(t *testing.T, rows ...modelRow) (*ChatService, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE ai_models (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			key          TEXT NOT NULL,
			display_name TEXT NOT NULL,
			model_slug   TEXT NOT NULL,
			enabled      INTEGER NOT NULL DEFAULT 1,
			sort_order   INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX uq_ai_models_key ON ai_models (key)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range rows {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO ai_models (key, display_name, model_slug, enabled, sort_order) VALUES (?, ?, ?, ?, ?)`,
			row.key, row.key, row.key+"/model", row.enabled, row.sortOrder,
		); err != nil {
			t.Fatal(err)
		}
	}

	repo := repository.NewAIModelsRepo(db)
	svc := NewAIModelsService(repo)
	return &ChatService{aiModelsSvc: svc}, ctx
}

func TestDefaultHostedLLMProviderOrderIncludesEnabledLocalAI(t *testing.T) {
	s, ctx := newTestChatServiceWithModelRows(t,
		modelRow{"claude", true, 0},
		modelRow{"localai", true, 1},
		modelRow{"openai", true, 2},
	)
	got := s.defaultHostedLLMProviderOrder(ctx)
	want := []string{"claude", "localai", "openai"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestDefaultHostedLLMProviderOrderExcludesDisabledLocalAI(t *testing.T) {
	s, ctx := newTestChatServiceWithModelRows(t,
		modelRow{"claude", true, 0},
		modelRow{"localai", false, 1},
		modelRow{"openai", true, 2},
	)
	got := s.defaultHostedLLMProviderOrder(ctx)
	want := []string{"claude", "openai"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if s.aiModelsSvc.LocalAIRowEnabled(ctx) {
		t.Fatal("expected LocalAIRowEnabled to report false for a disabled localai row")
	}
}

func TestListEnabledExcludesLocalAI(t *testing.T) {
	s, ctx := newTestChatServiceWithModelRows(t,
		modelRow{"claude", true, 0},
		modelRow{"localai", true, 1},
	)
	models, err := s.aiModelsSvc.ListEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range models {
		if m.Key == "localai" {
			t.Fatalf("ListEnabled should exclude localai, got %v", models)
		}
	}
}

func TestLocalAIRowEnabledFailsOpenWhenMissing(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "claude", "openai")
	if !s.aiModelsSvc.LocalAIRowEnabled(ctx) {
		t.Fatal("expected LocalAIRowEnabled to fail open (true) when no localai row exists")
	}
}

func TestNormalizeClassifierProvider(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "gemini", "claude", "deepseek", "openai")
	if got := s.normalizeClassifierProvider(ctx, "claude"); got != "claude" {
		t.Fatalf("got %q", got)
	}
	if got := s.normalizeClassifierProvider(ctx, "invalid"); got != DefaultClassifierProvider {
		t.Fatalf("got %q", got)
	}
}

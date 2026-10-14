package handler

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
)

func TestSetupChecklist_FollowsArchiveData(t *testing.T) {
	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{SQLitePath: filepath.Join(t.TempDir(), "archive.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.MigrateSQLite(ctx, db.Std); err != nil {
		t.Fatal(err)
	}
	appai.SeedCatalogForTests([]map[string]any{{"name": "search_chat_messages_globally", "description": "x"}})
	t.Cleanup(func() { appai.SeedCatalogForTests(nil) })

	configRepo := repository.NewConfigRepo(db.Std)
	h := NewSystemStatusHandler(db.Std, configRepo, nil, nil, nil, service.NewArchiveBackupService(db.Std, configRepo), nil, "local")

	byKey := func() map[string]setupItem {
		out := map[string]setupItem{}
		for _, it := range h.checklistItems(ctx) {
			out[it.Key] = it
		}
		return out
	}

	items := byKey()
	for _, key := range []string{"connect_ai", "subject_details", "first_import", "ai_tools", "name_people", "first_backup", "share"} {
		if _, ok := items[key]; !ok {
			t.Fatalf("missing checklist item %q", key)
		}
		if items[key].Done {
			t.Fatalf("%s should not be done on an empty archive", key)
		}
	}
	if items["name_people"].NotYet == "" || items["first_backup"].NotYet == "" {
		t.Fatal("people and backup steps should wait for data")
	}
	if items["ai_tools"].NotYet != "" {
		t.Fatal("ai_tools should be actionable when the catalog has tools")
	}

	for _, stmt := range []string{
		`INSERT INTO subject_configuration (subject_name, email_addresses) VALUES ('Alex', '["alex@example.com"]')`,
		`INSERT INTO messages (chat_session, message_date, sender_name, text, service) VALUES ('Sam', '2020-01-01 10:00:00', 'Sam', 'hi', 'SMS')`,
		`INSERT INTO contacts (name) VALUES ('Sam')`,
		`INSERT INTO face_clusters (face_count) VALUES (3)`,
		`INSERT INTO face_clusters (face_count, contact_id) VALUES (2, (SELECT id FROM contacts LIMIT 1))`,
	} {
		if _, err := db.Std.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	items = byKey()
	if !items["subject_details"].Done || !items["first_import"].Done || !items["name_people"].Done {
		t.Fatalf("expected subject/import/people done: %+v", items)
	}
	if items["subject_details"].Title != "Add Alex's email addresses and phone numbers" {
		t.Fatalf("subject title = %q", items["subject_details"].Title)
	}
	if items["first_backup"].NotYet != "" || items["first_backup"].Done {
		t.Fatalf("backup step should now be actionable and not done: %+v", items["first_backup"])
	}

	// Status rows never fail outright without optional services (no chat service, no user repo).
	checks := h.checks(ctx, httptest.NewRequest("GET", "/api/system-status", nil))
	found := map[string]statusCheck{}
	for _, c := range checks {
		found[c.Key] = c
	}
	if found["ai_connection"].Status != statusError {
		t.Fatalf("no AI configured should be an error: %+v", found["ai_connection"])
	}
	if c := found["tools_enabled"]; c.Status != statusError || c.Fix == nil || c.Fix.Action != fixEnableRecommended {
		t.Fatalf("no tools enabled should offer the recommended-tools fix: %+v", c)
	}
	if found["backup"].Status != statusWarn {
		t.Fatalf("never backed up with data should warn: %+v", found["backup"])
	}
	if worstStatus(checks) != statusError {
		t.Fatal("overall should be error")
	}
}

func TestHasListValue(t *testing.T) {
	for raw, want := range map[string]bool{
		"":                  false,
		"[]":                false,
		"null":              false,
		`[""]`:              false,
		"a@b.com":           true,
		`["a@b.com"]`:       true,
		"+61 400 000 000, ": true,
	} {
		if got := hasListValue(raw); got != want {
			t.Errorf("hasListValue(%q) = %v, want %v", raw, got, want)
		}
	}
}

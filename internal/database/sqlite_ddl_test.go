package database

import (
	"regexp"
	"strings"
	"testing"
)

func TestSqliteStatementsNoCurrentTextCorruption(t *testing.T) {
	for i, stmt := range sqliteStatements() {
		if strings.Contains(stmt, "CURRENT_TEXT") {
			t.Fatalf("statement %d contains CURRENT_TEXT (TIMESTAMP replace bug):\n%s", i, stmt)
		}
	}
}

func TestSqliteSessionsHasVisitorKeyHintColumn(t *testing.T) {
	for _, stmt := range sqliteStatements() {
		if !strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS sessions") {
			continue
		}
		if !strings.Contains(stmt, "visitor_key_hint_id") {
			t.Fatalf("sessions DDL must keep visitor_key_hint_id for repository queries:\n%s", stmt)
		}
		return
	}
	t.Fatal("sessions CREATE not found")
}

func TestSqliteSubjectConfigurationHasOwnerContactColumn(t *testing.T) {
	for _, stmt := range sqliteStatements() {
		if !strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS subject_configuration") {
			continue
		}
		if !strings.Contains(stmt, "subject_contact_id") {
			t.Fatalf("subject_configuration DDL must include subject_contact_id:\n%s", stmt)
		}
		return
	}
	t.Fatal("subject_configuration CREATE not found in sqliteStatements")
}

func TestRegexpTimestampInCurrentTimestamp(t *testing.T) {
	re := regexp.MustCompile(`(?i)\bTIMESTAMP\b`)
	for _, s := range []string{
		"DEFAULT CURRENT_TIMESTAMP",
		"DEFAULT (CURRENT_TIMESTAMP)",
	} {
		out := re.ReplaceAllString(s, "TEXT")
		if strings.Contains(out, "CURRENT_TEXT") {
			t.Fatalf("regexp corrupts: in=%q out=%q", s, out)
		}
	}
}

func TestPgDDLToSQLite_DoesNotCorruptCurrentTimestamp(t *testing.T) {
	const line = `created_at             TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,`
	out := pgDDLToSQLite(line)
	if strings.Contains(out, "CURRENT_TEXT") {
		t.Fatalf("pgDDLToSQLite corrupted CURRENT_TIMESTAMP: %q", out)
	}
	if !strings.Contains(out, "datetime('now')") {
		t.Fatalf("expected datetime('now') default, got: %q", out)
	}
}

func TestPgDDLToSQLite_ParenthesizedCurrentTimestamp(t *testing.T) {
	const line = `created_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP),`
	out := pgDDLToSQLite(line)
	if strings.Contains(out, "CURRENT_TEXT") {
		t.Fatalf("reTPlain must not replace TIMESTAMP inside CURRENT_TIMESTAMP: %q", out)
	}
}

func TestSqliteFacebookPostsDDLPreservesTimestampColumn(t *testing.T) {
	for _, stmt := range sqliteStatements() {
		if !strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS facebook_posts") {
			continue
		}
		if !strings.Contains(stmt, "timestamp") {
			t.Fatalf("facebook_posts SQLite DDL must keep a column named timestamp:\n%s", stmt)
		}
		return
	}
	t.Fatal("CREATE TABLE facebook_posts not found in sqliteStatements")
}

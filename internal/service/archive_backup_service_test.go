package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
)

// newBackupTestArchive creates a small WAL-mode archive file (with a sqlite-vec table, like a
// real archive) and returns an open single-connection pool, mirroring the main app's pool.
func newBackupTestArchive(t *testing.T) (*sql.DB, string) {
	t.Helper()
	database.EnsureSQLiteDriverLoaded()
	dir := t.TempDir()
	src := filepath.Join(dir, "My Archive.sqlite")
	db, err := sql.Open("sqlite3", config.SQLiteFileDSN(src))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE messages (id INTEGER PRIMARY KEY, text TEXT, user_id INTEGER)`,
		`INSERT INTO messages (text, user_id) VALUES ('hello', 2), ('world', 2)`,
		`CREATE VIRTUAL TABLE vec_test USING vec0(embedding float[4])`,
		`INSERT INTO vec_test(rowid, embedding) VALUES (1, '[1,2,3,4]')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db, src
}

func TestArchiveBackup_CopiesOpenArchive(t *testing.T) {
	ctx := context.Background()
	db, src := newBackupTestArchive(t)
	svc := NewArchiveBackupService(db, nil)

	got, err := svc.SourcePath(ctx)
	if err != nil || !samePath(got, src) {
		t.Fatalf("SourcePath = %q, %v; want %q", got, err, src)
	}

	dest := filepath.Join(filepath.Dir(src), "backup")
	dest, err = svc.ValidateDestination(ctx, dest)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(dest) != ".sqlite" {
		t.Fatalf("extension should be added, got %q", dest)
	}

	// Another connection holds an open write transaction while the backup runs (WAL mode):
	// the snapshot must still succeed without waiting for it.
	other, err := sql.Open("sqlite3", config.SQLiteFileDSN(src))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	tx, err := other.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO messages (text, user_id) VALUES ('uncommitted', 2)`); err != nil {
		t.Fatal(err)
	}
	res := svc.RunSync(ctx, dest)
	_ = tx.Rollback()
	if res.Error != "" {
		t.Fatalf("backup failed: %s", res.Error)
	}
	if res.SizeBytes <= 0 {
		t.Fatal("backup size should be recorded")
	}

	copyDB, err := sql.Open("sqlite3", config.SQLiteFileDSN(dest))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copyDB.Close() }()
	var n int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("messages in backup = %d, %v", n, err)
	}
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM vec_test`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("vec rows in backup = %d, %v", n, err)
	}

	matches, _ := filepath.Glob(dest + backupPartialSuffix + "*")
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestArchiveBackup_ReplacesExistingFileOnlyOnSuccess(t *testing.T) {
	ctx := context.Background()
	db, src := newBackupTestArchive(t)
	svc := NewArchiveBackupService(db, nil)
	dest := filepath.Join(filepath.Dir(src), "existing.sqlite")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := svc.RunSync(ctx, dest); res.Error != "" {
		t.Fatalf("backup failed: %s", res.Error)
	}
	b, _ := os.ReadFile(dest)
	if string(b) == "old" {
		t.Fatal("existing file should have been replaced by the new backup")
	}
}

func TestArchiveBackup_RejectsBadDestinations(t *testing.T) {
	ctx := context.Background()
	db, src := newBackupTestArchive(t)
	svc := NewArchiveBackupService(db, nil)
	for name, dest := range map[string]string{
		"empty":        "",
		"relative":     "backup.sqlite",
		"the archive":  src,
		"wal file":     src + "-wal",
		"missing dir":  filepath.Join(filepath.Dir(src), "nope", "b.sqlite"),
		"is directory": filepath.Dir(src),
	} {
		if _, err := svc.ValidateDestination(ctx, dest); err == nil {
			t.Errorf("%s: expected an error for %q", name, dest)
		}
	}
}

func TestBackupReminderDue(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-5 * 24 * time.Hour)
	old := now.Add(-40 * 24 * time.Hour)
	cases := []struct {
		name    string
		hasData bool
		last    *time.Time
		snooze  time.Time
		want    bool
	}{
		{"empty archive", false, nil, time.Time{}, false},
		{"never backed up", true, nil, time.Time{}, true},
		{"recent backup", true, &recent, time.Time{}, false},
		{"old backup", true, &old, time.Time{}, true},
		{"snoozed", true, &old, now.Add(time.Hour), false},
		{"snooze expired", true, &old, now.Add(-time.Hour), true},
	}
	for _, c := range cases {
		if got := backupReminderDue(c.hasData, c.last, c.snooze, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestInspectRestoreCandidate(t *testing.T) {
	ctx := context.Background()
	db, src := newBackupTestArchive(t)
	for _, stmt := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE app_configuration (key TEXT, value TEXT)`,
		`CREATE TABLE emails (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE media_items (id INTEGER PRIMARY KEY)`,
		`INSERT INTO emails DEFAULT VALUES`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	svc := NewArchiveBackupService(db, nil)
	dir := filepath.Dir(src)

	backup := filepath.Join(dir, "good-backup.sqlite")
	if res := svc.RunSync(ctx, backup); res.Error != "" {
		t.Fatalf("backup failed: %s", res.Error)
	}
	cand, err := svc.InspectRestoreCandidate(ctx, backup)
	if err != nil {
		t.Fatalf("good backup rejected: %v", err)
	}
	if cand.Messages != 2 || cand.Emails != 1 || cand.Photos != 0 || !samePath(cand.ArchivePath, src) {
		t.Fatalf("unexpected candidate %+v", cand)
	}
	// Inspecting must not leave files behind next to the backup.
	for _, sfx := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(backup + sfx); err == nil {
			t.Fatalf("inspection created %s", backup+sfx)
		}
	}

	notSQLite := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notSQLite, []byte("hello, this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	otherDB := filepath.Join(dir, "other.sqlite")
	odb, err := sql.Open("sqlite3", config.SQLiteFileDSN(otherDB))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := odb.Exec(`CREATE TABLE things (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = odb.Close()
	withWAL := filepath.Join(dir, "live-copy.sqlite")
	if res := svc.RunSync(ctx, withWAL); res.Error != "" {
		t.Fatal(res.Error)
	}
	if err := os.WriteFile(withWAL+"-wal", []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"empty":          "",
		"relative":       "backup.sqlite",
		"missing":        filepath.Join(dir, "nope.sqlite"),
		"folder":         dir,
		"open archive":   src,
		"archive wal":    src + "-wal",
		"not sqlite":     notSQLite,
		"not an archive": otherDB,
		"pending wal":    withWAL,
	} {
		if _, err := svc.InspectRestoreCandidate(ctx, path); err == nil {
			t.Errorf("%s: expected rejection for %q", name, path)
		}
	}
}

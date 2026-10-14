package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/repository"
)

// Archive backup — a consistent, self-contained copy of the main archive SQLite file.
//
// The copy is made with SQLite's `VACUUM INTO` on a *separate* connection to the same file, so
// the app's single main connection (internal/database/db.go) is never blocked, and (because the
// archive runs in WAL mode — see the MCP server notes in CLAUDE.md) readers don't block writers
// either. The snapshot is written to a temporary file beside the destination, checked with
// `PRAGMA quick_check`, and only then renamed into place, so a failed or interrupted backup never
// leaves a half-written file under the chosen name or clobbers an older good backup.
//
// Encrypted items stay encrypted in the copy (same keyring), and photos imported in "link to
// images on disk" mode are not inside the database, so they are not part of the backup.

// Config keys (app_configuration, per archive owner).
const (
	LastBackupAtConfigKey         = "last_backup_at_v1"
	LastBackupPathConfigKey       = "last_backup_path_v1"
	BackupReminderSnoozeConfigKey = "backup_reminder_snoozed_until_v1"
	BackupReminderAfter           = 30 * 24 * time.Hour
	backupPartialSuffix           = ".partial"
	backupQuickCheckOK            = "ok"
)

// ErrBackupInProgress is returned when a backup is already running.
var ErrBackupInProgress = errors.New("a backup is already in progress")

// BackupResult describes the outcome of the most recent backup in this process.
type BackupResult struct {
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error,omitempty"`
}

// ArchiveBackupService creates archive backups. One backup runs at a time per process.
type ArchiveBackupService struct {
	pool       *sql.DB
	configRepo *repository.ConfigRepo

	mu        sync.Mutex
	running   bool
	startedAt time.Time
	dest      string
	last      *BackupResult
}

// NewArchiveBackupService creates an ArchiveBackupService.
func NewArchiveBackupService(pool *sql.DB, configRepo *repository.ConfigRepo) *ArchiveBackupService {
	return &ArchiveBackupService{pool: pool, configRepo: configRepo}
}

// SourcePath returns the absolute path of the open main archive file.
func (s *ArchiveBackupService) SourcePath(ctx context.Context) (string, error) {
	if s.pool == nil {
		return "", errors.New("no archive is open")
	}
	rows, err := s.pool.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", fmt.Errorf("read database list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", err
		}
		if name == "main" {
			if strings.TrimSpace(file) == "" {
				return "", errors.New("the archive is an in-memory database")
			}
			return file, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", errors.New("main database not found")
}

// SuggestedFileName returns e.g. "MyArchive-backup-2026-10-13-1405.sqlite".
func (s *ArchiveBackupService) SuggestedFileName(ctx context.Context, now time.Time) string {
	base := "archive"
	if src, err := s.SourcePath(ctx); err == nil {
		b := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		if strings.TrimSpace(b) != "" {
			base = b
		}
	}
	return fmt.Sprintf("%s-backup-%s.sqlite", base, now.Format("2006-01-02-1504"))
}

// ValidateDestination checks a requested destination path and returns it cleaned.
func (s *ArchiveBackupService) ValidateDestination(ctx context.Context, dest string) (string, error) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return "", errors.New("choose where to save the backup")
	}
	if !filepath.IsAbs(dest) {
		return "", errors.New("the backup location must be a full path")
	}
	dest = filepath.Clean(dest)
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		return "", errors.New("the backup location is a folder, not a file")
	}
	if filepath.Ext(dest) == "" {
		dest += ".sqlite"
		if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
			return "", errors.New("the backup location is a folder, not a file")
		}
	}
	dir := filepath.Dir(dest)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("folder does not exist: %s", dir)
	}
	src, err := s.SourcePath(ctx)
	if err != nil {
		return "", err
	}
	if samePath(src, dest) || samePath(src+"-wal", dest) || samePath(src+"-shm", dest) || samePath(src+"-journal", dest) {
		return "", errors.New("the backup can't overwrite the archive itself — choose a different file")
	}
	return dest, nil
}

func samePath(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if aa, err := filepath.Abs(ca); err == nil {
		ca = aa
	}
	if bb, err := filepath.Abs(cb); err == nil {
		cb = bb
	}
	return strings.EqualFold(ca, cb)
}

// Start launches a backup to dest in the background for uid. dest must already be validated.
func (s *ArchiveBackupService) Start(uid int64, dest string) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrBackupInProgress
	}
	s.running = true
	s.startedAt = time.Now().UTC()
	s.dest = dest
	s.mu.Unlock()

	go func() {
		ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
		res := s.run(ctx, dest)
		s.mu.Lock()
		s.running = false
		s.last = res
		s.mu.Unlock()
		if res.Error == "" {
			s.recordSuccess(ctx, res)
			slog.Info("archive backup completed", "path", res.Path, "bytes", res.SizeBytes, "took", res.FinishedAt.Sub(res.StartedAt))
		} else {
			slog.Warn("archive backup failed", "path", dest, "err", res.Error)
		}
	}()
	return nil
}

// RunSync performs a backup synchronously (used by tests).
func (s *ArchiveBackupService) RunSync(ctx context.Context, dest string) *BackupResult {
	return s.run(ctx, dest)
}

func (s *ArchiveBackupService) run(ctx context.Context, dest string) *BackupResult {
	res := &BackupResult{Path: dest, StartedAt: time.Now().UTC()}
	fail := func(err error) *BackupResult {
		res.Error = err.Error()
		res.FinishedAt = time.Now().UTC()
		return res
	}

	src, err := s.SourcePath(ctx)
	if err != nil {
		return fail(err)
	}
	partial := fmt.Sprintf("%s%s-%d", dest, backupPartialSuffix, time.Now().UnixNano())
	defer func() { _ = os.Remove(partial) }()

	// Separate connection: never ties up the app's single main connection.
	conn, err := sql.Open("sqlite3", config.SQLiteFileDSN(src))
	if err != nil {
		return fail(fmt.Errorf("open archive for backup: %w", err))
	}
	conn.SetMaxOpenConns(1)
	_, err = conn.ExecContext(ctx, `VACUUM INTO ?`, partial)
	_ = conn.Close()
	if err != nil {
		return fail(fmt.Errorf("copy archive: %w", err))
	}

	if err := quickCheck(ctx, partial); err != nil {
		return fail(err)
	}

	// Replace any existing file at dest only now that the new copy is known good.
	if _, err := os.Stat(dest); err == nil {
		if err := os.Remove(dest); err != nil {
			return fail(fmt.Errorf("replace existing file: %w", err))
		}
	}
	if err := os.Rename(partial, dest); err != nil {
		return fail(fmt.Errorf("save backup: %w", err))
	}
	if fi, err := os.Stat(dest); err == nil {
		res.SizeBytes = fi.Size()
	}
	res.FinishedAt = time.Now().UTC()
	return res
}

func quickCheck(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite3", config.SQLiteFileDSN(path))
	if err != nil {
		return fmt.Errorf("open backup for checking: %w", err)
	}
	defer func() { _ = db.Close() }()
	var out string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&out); err != nil {
		return fmt.Errorf("check backup: %w", err)
	}
	if out != backupQuickCheckOK {
		return fmt.Errorf("backup failed its integrity check: %s", out)
	}
	return nil
}

func (s *ArchiveBackupService) recordSuccess(ctx context.Context, res *BackupResult) {
	if s.configRepo == nil {
		return
	}
	f := false
	at := res.FinishedAt.Format(time.RFC3339)
	path := res.Path
	descAt := "When the archive was last backed up"
	descPath := "Where the archive was last backed up to"
	if _, err := s.configRepo.Upsert(ctx, LastBackupAtConfigKey, &at, &f, &descAt); err != nil {
		slog.Warn("record backup time failed", "err", err)
	}
	if _, err := s.configRepo.Upsert(ctx, LastBackupPathConfigKey, &path, &f, &descPath); err != nil {
		slog.Warn("record backup path failed", "err", err)
	}
}

// BackupStatus is the GET /api/backup/status payload.
type BackupStatus struct {
	Running           bool          `json:"running"`
	RunningSince      *time.Time    `json:"running_since,omitempty"`
	RunningPath       string        `json:"running_path,omitempty"`
	Last              *BackupResult `json:"last,omitempty"`
	LastBackupAt      *time.Time    `json:"last_backup_at,omitempty"`
	LastBackupPath    string        `json:"last_backup_path,omitempty"`
	ReminderDue       bool          `json:"reminder_due"`
	SuggestedFileName string        `json:"suggested_file_name"`
	ArchiveSizeBytes  int64         `json:"archive_size_bytes"`
	ArchivePath       string        `json:"archive_path,omitempty"`
}

// Status reports the current/last backup and whether the owner should be reminded.
// hasData tells whether the archive holds anything worth backing up yet.
func (s *ArchiveBackupService) Status(ctx context.Context, hasData bool, now time.Time) BackupStatus {
	st := BackupStatus{SuggestedFileName: s.SuggestedFileName(ctx, now.Local())}
	s.mu.Lock()
	if s.running {
		st.Running = true
		t := s.startedAt
		st.RunningSince = &t
		st.RunningPath = s.dest
	}
	if s.last != nil {
		cp := *s.last
		st.Last = &cp
	}
	s.mu.Unlock()

	if src, err := s.SourcePath(ctx); err == nil {
		st.ArchivePath = src
		if fi, err := os.Stat(src); err == nil {
			st.ArchiveSizeBytes = fi.Size()
		}
	}

	var snoozedUntil time.Time
	if s.configRepo != nil {
		if raw, err := s.configRepo.GetValueByKey(ctx, LastBackupAtConfigKey); err == nil && raw != nil {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw)); err == nil {
				st.LastBackupAt = &t
			}
		}
		if raw, err := s.configRepo.GetValueByKey(ctx, LastBackupPathConfigKey); err == nil && raw != nil {
			st.LastBackupPath = *raw
		}
		if raw, err := s.configRepo.GetValueByKey(ctx, BackupReminderSnoozeConfigKey); err == nil && raw != nil {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw)); err == nil {
				snoozedUntil = t
			}
		}
	}
	st.ReminderDue = backupReminderDue(hasData, st.LastBackupAt, snoozedUntil, now) && !st.Running
	return st
}

// backupReminderDue: remind when there is data and no backup in BackupReminderAfter, unless snoozed.
func backupReminderDue(hasData bool, lastBackup *time.Time, snoozedUntil, now time.Time) bool {
	if !hasData {
		return false
	}
	if !snoozedUntil.IsZero() && now.Before(snoozedUntil) {
		return false
	}
	if lastBackup == nil {
		return true
	}
	return now.Sub(*lastBackup) >= BackupReminderAfter
}

// Snooze hides the reminder for the given number of days.
func (s *ArchiveBackupService) Snooze(ctx context.Context, days int, now time.Time) error {
	if s.configRepo == nil {
		return nil
	}
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}
	until := now.Add(time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339)
	f := false
	desc := "Backup reminder hidden until this time"
	_, err := s.configRepo.Upsert(ctx, BackupReminderSnoozeConfigKey, &until, &f, &desc)
	return err
}

// ArchiveHasData reports whether the archive file holds any messages, emails or photos. The
// backup copies the whole file, so this is deliberately not user-scoped.
func (s *ArchiveBackupService) ArchiveHasData(ctx context.Context) bool {
	if s.pool == nil {
		return false
	}
	for _, table := range []string{"messages", "emails", "media_items"} {
		var one int
		if err := s.pool.QueryRowContext(ctx, `SELECT 1 FROM `+table+` LIMIT 1`).Scan(&one); err == nil {
			return true
		}
	}
	return false
}

// ── Restore ──────────────────────────────────────────────────────────────────
//
// Restoring swaps the archive file itself, which can't happen while this server (and the MCP
// tools server) have it open, so the swap is done by the desktop shell (electron/main.js,
// "restore-archive"): it stops both servers, moves the current archive aside as a safety copy,
// copies the backup into place and restarts. The server's part is InspectRestoreCandidate —
// checking, before anything is stopped, that the chosen file really is a healthy archive.

// RestoreCandidate describes a backup file the owner has picked to restore.
type RestoreCandidate struct {
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes"`
	ModifiedAt  time.Time `json:"modified_at"`
	ArchivePath string    `json:"archive_path"`
	Messages    int64     `json:"messages"`
	Emails      int64     `json:"emails"`
	Photos      int64     `json:"photos"`
}

// sqliteFileHeader is the first 16 bytes of every SQLite 3 database file.
const sqliteFileHeader = "SQLite format 3\x00"

// requiredArchiveTables must all exist for a file to be treated as a Digital Museum archive.
var requiredArchiveTables = []string{"users", "app_configuration", "messages", "emails", "media_items"}

// InspectRestoreCandidate checks that path is a readable, intact Digital Museum archive that is
// not the archive currently open, and summarises what it contains. It opens the file read-only.
func (s *ArchiveBackupService) InspectRestoreCandidate(ctx context.Context, path string) (*RestoreCandidate, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("choose a backup file to restore")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("the backup location must be a full path")
	}
	path = filepath.Clean(path)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("can't open that file: %w", err)
	}
	if fi.IsDir() {
		return nil, errors.New("that's a folder — choose the backup file inside it")
	}
	src, err := s.SourcePath(ctx)
	if err != nil {
		return nil, err
	}
	if samePath(src, path) || samePath(src+"-wal", path) || samePath(src+"-shm", path) || samePath(src+"-journal", path) {
		return nil, errors.New("that's the archive you have open now — choose a backup file instead")
	}
	if err := checkSQLiteHeader(path); err != nil {
		return nil, err
	}
	// A copy taken from a running archive can have unsaved changes in a "-wal" file beside it;
	// only the main file is restored, so those changes would be lost. App-made backups never
	// have one.
	if wfi, err := os.Stat(path + "-wal"); err == nil && wfi.Size() > 0 {
		return nil, errors.New("that file has unsaved changes in a \"-wal\" file next to it (it was copied while Digital Museum was running) — choose a backup made with Back Up Archive")
	}

	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	var check string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil {
		return nil, fmt.Errorf("couldn't read the backup: %w", err)
	}
	if check != backupQuickCheckOK {
		return nil, fmt.Errorf("the backup file is damaged (integrity check: %s)", check)
	}
	for _, table := range requiredArchiveTables {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
			return nil, fmt.Errorf("couldn't read the backup: %w", err)
		}
		if n == 0 {
			return nil, errors.New("that file isn't a Digital Museum archive")
		}
	}

	out := &RestoreCandidate{
		Path:        path,
		SizeBytes:   fi.Size(),
		ModifiedAt:  fi.ModTime().UTC(),
		ArchivePath: src,
	}
	// Whole-file counts (not user-scoped): the restore replaces the whole file.
	for table, dst := range map[string]*int64{"messages": &out.Messages, "emails": &out.Emails, "media_items": &out.Photos} {
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(dst)
	}
	return out, nil
}

// Busy reports whether a backup is currently being written.
func (s *ArchiveBackupService) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func checkSQLiteHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("can't open that file: %w", err)
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, len(sqliteFileHeader))
	if _, err := io.ReadFull(f, buf); err != nil || string(buf) != sqliteFileHeader {
		return errors.New("that file isn't a Digital Museum archive")
	}
	return nil
}

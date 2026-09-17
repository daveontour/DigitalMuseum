package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var archiveNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// ResolveAdminSQLitePath returns the billing/admin SQLite file path.
// When ADMIN_SQLITE_PATH is unset, the default is <executableDir>/data/admin.sqlite.
// Absolute env values are used as-is; relative values are resolved against the executable directory.
// If the configured path is an existing directory (a common misconfiguration), admin.sqlite is
// appended so SQLite is never pointed at a folder.
func ResolveAdminSQLitePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exe)
	raw := strings.TrimSpace(os.Getenv("ADMIN_SQLITE_PATH"))
	var p string
	if raw == "" {
		p = filepath.Join(exeDir, "data", "admin.sqlite")
	} else if filepath.IsAbs(raw) {
		p = filepath.Clean(raw)
	} else {
		p = filepath.Clean(filepath.Join(exeDir, raw))
	}
	return ensureAdminSQLiteFilePath(p), nil
}

func ensureAdminSQLiteFilePath(p string) string {
	st, err := os.Stat(p)
	if err == nil && st.IsDir() {
		return filepath.Join(p, "admin.sqlite")
	}
	// Path does not exist yet: if it looks like a directory (no extension), still append.
	if err != nil && filepath.Ext(p) == "" {
		return filepath.Join(p, "admin.sqlite")
	}
	return p
}

// AdminDataDir returns the directory containing the admin/billing SQLite file.
func AdminDataDir() (string, error) {
	p, err := ResolveAdminSQLitePath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

// DefaultNewArchiveSQLitePath returns a new archive path under AdminDataDir from a display name.
func DefaultNewArchiveSQLitePath(baseName string) (string, error) {
	dir, err := AdminDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, archiveSlug(baseName)+".sqlite"), nil
}

func archiveSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = archiveNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "archive"
	}
	return s
}

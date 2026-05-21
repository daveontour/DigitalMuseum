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
func ResolveAdminSQLitePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exe)
	raw := strings.TrimSpace(os.Getenv("ADMIN_SQLITE_PATH"))
	if raw == "" {
		return filepath.Join(exeDir, "data", "admin.sqlite"), nil
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Clean(filepath.Join(exeDir, raw)), nil
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

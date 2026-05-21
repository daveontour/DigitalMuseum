package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAdminSQLitePath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeDir := filepath.Dir(exe)
	defaultWant := filepath.Join(exeDir, "data", "admin.sqlite")

	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("ADMIN_SQLITE_PATH", "")
		got, err := ResolveAdminSQLitePath()
		if err != nil {
			t.Fatal(err)
		}
		if got != defaultWant {
			t.Fatalf("got %q want %q", got, defaultWant)
		}
	})

	t.Run("absolute override", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "custom.sqlite")
		t.Setenv("ADMIN_SQLITE_PATH", abs)
		got, err := ResolveAdminSQLitePath()
		if err != nil {
			t.Fatal(err)
		}
		if got != abs {
			t.Fatalf("got %q want %q", got, abs)
		}
	})

	t.Run("relative override under exe dir", func(t *testing.T) {
		t.Setenv("ADMIN_SQLITE_PATH", "./data/custom.sqlite")
		got, err := ResolveAdminSQLitePath()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(exeDir, "data", "custom.sqlite")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("admin data dir", func(t *testing.T) {
		t.Setenv("ADMIN_SQLITE_PATH", "")
		dir, err := AdminDataDir()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(exeDir, "data")
		if dir != want {
			t.Fatalf("got %q want %q", dir, want)
		}
	})

	t.Run("default new archive path", func(t *testing.T) {
		t.Setenv("ADMIN_SQLITE_PATH", "")
		got, err := DefaultNewArchiveSQLitePath("Jane Doe")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(exeDir, "data", "jane-doe.sqlite")
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
}

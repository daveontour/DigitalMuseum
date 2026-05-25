package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

func TestLoadAdminCredentialsFromDotEnvOverridesOS(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("ADMIN_EMAIL=file@example.com\nADMIN_PASSWORD=FileSecret123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADMIN_EMAIL", "stale@example.com")
	t.Setenv("ADMIN_PASSWORD", "stale-secret")

	origWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := godotenv.Overload(envPath); err != nil {
		t.Fatal(err)
	}

	if got := loadAdminEmail(); got != "file@example.com" {
		t.Fatalf("email = %q, want file@example.com", got)
	}
	if got := loadAdminPassword(); got != "FileSecret123" {
		t.Fatalf("password mismatch: got len %d", len(got))
	}
}

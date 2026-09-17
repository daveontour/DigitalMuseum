package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDotEnvFileRecordsSource(t *testing.T) {
	resetEnvSources()
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("PAGE_TITLE=FromFile\nADMIN_PASSWORD=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAGE_TITLE", "FromProcess")
	recordProcessEnvBaseline()
	applyDotEnvFile(envPath)

	if got := os.Getenv("PAGE_TITLE"); got != "FromFile" {
		t.Fatalf("PAGE_TITLE=%q want FromFile", got)
	}
	src := sourceForKey("PAGE_TITLE")
	if !strings.Contains(src, ".env") {
		t.Fatalf("source=%q want .env path", src)
	}

	cfg := &Config{
		DB:  DatabaseConfig{BillingSQLitePath: filepath.Join(dir, "admin.sqlite")},
		App: AppConfig{PageTitle: "FromFile", DeploymentNature: "web", TemplatesDir: "t", AssetStaticDir: "s"},
	}
	report := SnapshotLoadedConfig(cfg)
	if len(report.FilesLoaded) != 1 {
		t.Fatalf("files_loaded=%v", report.FilesLoaded)
	}
	var pageTitle, adminPass *LoadedConfigItem
	for i := range report.Items {
		it := &report.Items[i]
		if it.Key == "PAGE_TITLE" {
			pageTitle = it
		}
		if it.Key == "ADMIN_PASSWORD" {
			adminPass = it
		}
	}
	if pageTitle == nil || pageTitle.Value != "FromFile" {
		t.Fatalf("PAGE_TITLE item=%+v", pageTitle)
	}
	if adminPass == nil || adminPass.Value != "(set)" || !adminPass.Secret {
		t.Fatalf("ADMIN_PASSWORD item=%+v", adminPass)
	}
}

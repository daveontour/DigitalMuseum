package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExcludeFolderPatterns(t *testing.T) {
	root := t.TempDir()
	keepDir := filepath.Join(root, "keep")
	excludeDir := filepath.Join(root, ".photostructure", "cache")

	for _, dir := range []string{keepDir, excludeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(keepDir, "visible.txt"), "keep")
	writeFile(t, filepath.Join(excludeDir, "hidden.txt"), "skip")

	result, err := Scan(root, root, ScanOptions{
		Dir1ExcludePatterns: []string{`\.photostructure`},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if result.Dir1Files != 1 {
		t.Fatalf("expected 1 scanned file, got %d", result.Dir1Files)
	}
	if len(result.Duplicates) != 0 {
		t.Fatalf("expected no duplicates, got %d", len(result.Duplicates))
	}
}

func TestInvalidExcludePattern(t *testing.T) {
	root := t.TempDir()
	_, err := Scan(root, root, ScanOptions{
		Dir1ExcludePatterns: []string{"["},
	}, nil)
	if err == nil {
		t.Fatal("expected invalid pattern error")
	}
}

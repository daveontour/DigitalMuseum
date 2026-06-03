package delete

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFiles(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "one")
	dir2 := filepath.Join(root, "two")
	file1 := filepath.Join(dir1, "keep.txt")
	file2 := filepath.Join(dir2, "remove.txt")
	outside := filepath.Join(root, "outside.txt")

	for _, dir := range []string{dir1, dir2} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(file1, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Files([]string{dir1, dir2}, []string{file2, outside})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != file2 {
		t.Fatalf("unexpected deleted files: %+v", result.Deleted)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("expected one failed deletion, got %d", len(result.Failed))
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Fatal("expected file2 to be deleted")
	}
	if _, err := os.Stat(file1); err != nil {
		t.Fatal("expected file1 to remain")
	}
}

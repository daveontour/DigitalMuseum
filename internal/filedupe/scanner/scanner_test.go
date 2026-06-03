package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDuplicates(t *testing.T) {
	dir1Map := map[fileKey][]string{
		{name: "a.txt", size: 10}: {"/dir1/a.txt"},
		{name: "b.txt", size: 20}: {"/dir1/b.txt"},
	}
	dir2Map := map[fileKey][]string{
		{name: "a.txt", size: 10}: {"/dir2/sub/a.txt"},
		{name: "c.txt", size: 30}: {"/dir2/c.txt"},
	}

	dups := findDuplicates(dir1Map, dir2Map)
	if len(dups) != 1 {
		t.Fatalf("expected 1 duplicate, got %d", len(dups))
	}
	if dups[0].FileName != "a.txt" || dups[0].Size != 10 {
		t.Fatalf("unexpected duplicate: %+v", dups[0])
	}
}

func TestFindDuplicatesWithin(t *testing.T) {
	files := map[fileKey][]string{
		{name: "a.txt", size: 10}: {
			"/dir/sub1/a.txt",
			"/dir/sub2/a.txt",
			"/dir/sub3/a.txt",
		},
		{name: "b.txt", size: 20}: {"/dir/b.txt"},
	}

	dups := findDuplicatesWithin(files)
	if len(dups) != 3 {
		t.Fatalf("expected 3 duplicate pairs, got %d", len(dups))
	}

	for _, dup := range dups {
		if pathsEqual(dup.Path1, dup.Path2) {
			t.Fatalf("file matched itself: %+v", dup)
		}
	}
}

func TestScanSameDirectory(t *testing.T) {
	root := t.TempDir()
	sub1 := filepath.Join(root, "alpha")
	sub2 := filepath.Join(root, "beta")

	for _, dir := range []string{sub1, sub2} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(sub1, "match.txt"), "hello")
	writeFile(t, filepath.Join(sub2, "match.txt"), "hello")
	writeFile(t, filepath.Join(root, "unique.txt"), "only one")

	result, err := Scan(root, root, ScanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Duplicates) != 1 {
		t.Fatalf("expected 1 duplicate, got %d", len(result.Duplicates))
	}
	if result.Dir1Files != 3 || result.Dir2Files != 3 {
		t.Fatalf("unexpected file counts: dir1=%d dir2=%d", result.Dir1Files, result.Dir2Files)
	}
}

func TestScanIntegration(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "one")
	dir2 := filepath.Join(root, "two")

	for _, dir := range []string{dir1, dir2} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(dir1, "match.txt"), "hello")
	writeFile(t, filepath.Join(dir2, "nested", "match.txt"), "hello")
	writeFile(t, filepath.Join(dir1, "unique.txt"), "only in one")
	writeFile(t, filepath.Join(dir2, "other.txt"), "different name")

	var updates []ProgressUpdate
	result, err := Scan(dir1, dir2, ScanOptions{}, func(u ProgressUpdate) {
		updates = append(updates, u)
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Duplicates) != 1 {
		t.Fatalf("expected 1 duplicate, got %d", len(result.Duplicates))
	}
	if len(updates) == 0 {
		t.Fatal("expected progress updates")
	}
}

func TestScanMatchesCopySuffixesWhenSizeMatches(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "one")
	dir2 := filepath.Join(root, "two")

	for _, dir := range []string{dir1, dir2} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(dir1, "photo.jpg"), "same-size-content")
	writeFile(t, filepath.Join(dir2, "photo (1).jpg"), "same-size-content")
	writeFile(t, filepath.Join(dir2, "photo-copy.jpg"), "different-size")

	result, err := Scan(dir1, dir2, ScanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Duplicates) != 1 {
		t.Fatalf("expected 1 duplicate by copy suffix, got %d", len(result.Duplicates))
	}
}

func TestScanDifferentDirectoriesIncludesWithinDirectoryDuplicates(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "one")
	dir2 := filepath.Join(root, "two")

	for _, dir := range []string{dir1, dir2} {
		if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "b"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Between directories duplicate.
	writeFile(t, filepath.Join(dir1, "a", "shared.txt"), "cross")
	writeFile(t, filepath.Join(dir2, "a", "shared.txt"), "cross")

	// Within directory 1 duplicate.
	writeFile(t, filepath.Join(dir1, "a", "inside1.txt"), "inside-one")
	writeFile(t, filepath.Join(dir1, "b", "inside1.txt"), "inside-one")

	// Within directory 2 duplicate.
	writeFile(t, filepath.Join(dir2, "a", "inside2.txt"), "inside-two")
	writeFile(t, filepath.Join(dir2, "b", "inside2.txt"), "inside-two")

	result, err := Scan(dir1, dir2, ScanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Duplicates) != 3 {
		t.Fatalf("expected 3 duplicates (cross + within each dir), got %d", len(result.Duplicates))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

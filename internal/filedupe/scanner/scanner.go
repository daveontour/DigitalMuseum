package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type ScanOptions struct {
	Dir1ExcludePatterns []string
	Dir2ExcludePatterns []string
}

type FileEntry struct {
	Name string
	Size int64
	Path string
}

type fileKey struct {
	name string
	size int64
}

var copySuffixPattern = regexp.MustCompile(`(?i)\s*(\(\d+\)|-\s*copy(?:\s*\(\d+\))?|copy(?:\s*\(\d+\))?)$`)

type ProgressUpdate struct {
	Phase        string `json:"phase"`
	FilesScanned int    `json:"filesScanned"`
	CurrentPath  string `json:"currentPath,omitempty"`
	Message      string `json:"message,omitempty"`
}

type Duplicate struct {
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	Path1    string `json:"path1"`
	Path2    string `json:"path2"`
}

type Result struct {
	Dir1Files    int         `json:"dir1Files"`
	Dir2Files    int         `json:"dir2Files"`
	Duplicates   []Duplicate `json:"duplicates"`
	ScanDuration string      `json:"scanDuration"`
}

func ValidateDirectory(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("directory path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", path)
	}
	return nil
}

func Scan(dir1, dir2 string, opts ScanOptions, onProgress func(ProgressUpdate)) (Result, error) {
	dir1 = filepath.Clean(strings.TrimSpace(dir1))
	dir2 = filepath.Clean(strings.TrimSpace(dir2))

	if err := ValidateDirectory(dir1); err != nil {
		return Result{}, fmt.Errorf("directory 1: %w", err)
	}
	if err := ValidateDirectory(dir2); err != nil {
		return Result{}, fmt.Errorf("directory 2: %w", err)
	}

	dir1Exclude, err := compileExcludePatterns(opts.Dir1ExcludePatterns)
	if err != nil {
		return Result{}, fmt.Errorf("directory 1 exclude patterns: %w", err)
	}
	dir2Exclude, err := compileExcludePatterns(opts.Dir2ExcludePatterns)
	if err != nil {
		return Result{}, fmt.Errorf("directory 2 exclude patterns: %w", err)
	}

	emit := func(update ProgressUpdate) {
		if onProgress != nil {
			onProgress(update)
		}
	}

	emit(ProgressUpdate{
		Phase:   "scanning_dir1",
		Message: fmt.Sprintf("Scanning %s", dir1),
	})

	sameDir, err := directoriesEqual(dir1, dir2)
	if err != nil {
		return Result{}, fmt.Errorf("compare directories: %w", err)
	}

	if sameDir {
		exclude := mergeExcludePatterns(dir1Exclude, dir2Exclude)
		fileMap, fileCount, err := walkDirectory(dir1, exclude, func(path string, count int) {
			emit(ProgressUpdate{
				Phase:        "scanning_dir1",
				FilesScanned: count,
				CurrentPath:  path,
			})
		})
		if err != nil {
			return Result{}, fmt.Errorf("scanning directory: %w", err)
		}

		emit(ProgressUpdate{
			Phase:        "scanning_dir1",
			FilesScanned: fileCount,
			Message:      fmt.Sprintf("Finished scanning directory (%d files)", fileCount),
		})

		emit(ProgressUpdate{
			Phase:   "comparing",
			Message: "Comparing files by name and size within the directory",
		})

		duplicates := findDuplicatesWithin(fileMap)

		emit(ProgressUpdate{
			Phase:   "complete",
			Message: fmt.Sprintf("Found %d duplicate pair(s)", len(duplicates)),
		})

		return Result{
			Dir1Files:  fileCount,
			Dir2Files:  fileCount,
			Duplicates: duplicates,
		}, nil
	}

	dir1Map, dir1Count, err := walkDirectory(dir1, dir1Exclude, func(path string, count int) {
		emit(ProgressUpdate{
			Phase:        "scanning_dir1",
			FilesScanned: count,
			CurrentPath:  path,
		})
	})
	if err != nil {
		return Result{}, fmt.Errorf("scanning directory 1: %w", err)
	}

	emit(ProgressUpdate{
		Phase:        "scanning_dir1",
		FilesScanned: dir1Count,
		Message:      fmt.Sprintf("Finished scanning directory 1 (%d files)", dir1Count),
	})

	emit(ProgressUpdate{
		Phase:   "scanning_dir2",
		Message: fmt.Sprintf("Scanning %s", dir2),
	})

	dir2Map, dir2Count, err := walkDirectory(dir2, dir2Exclude, func(path string, count int) {
		emit(ProgressUpdate{
			Phase:        "scanning_dir2",
			FilesScanned: count,
			CurrentPath:  path,
		})
	})
	if err != nil {
		return Result{}, fmt.Errorf("scanning directory 2: %w", err)
	}

	emit(ProgressUpdate{
		Phase:        "scanning_dir2",
		FilesScanned: dir2Count,
		Message:      fmt.Sprintf("Finished scanning directory 2 (%d files)", dir2Count),
	})

	emit(ProgressUpdate{
		Phase:   "comparing",
		Message: "Comparing files by name and size (within and between directories)",
	})

	duplicates := findDuplicates(dir1Map, dir2Map)
	duplicates = append(duplicates, findDuplicatesWithin(dir1Map)...)
	duplicates = append(duplicates, findDuplicatesWithin(dir2Map)...)

	emit(ProgressUpdate{
		Phase:   "complete",
		Message: fmt.Sprintf("Found %d duplicate pair(s)", len(duplicates)),
	})

	return Result{
		Dir1Files:  dir1Count,
		Dir2Files:  dir2Count,
		Duplicates: duplicates,
	}, nil
}

func walkDirectory(root string, exclude excludePatterns, onFile func(path string, count int)) (map[fileKey][]string, int, error) {
	files := make(map[fileKey][]string)
	count := 0
	root = filepath.Clean(root)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && exclude.matchesFolder(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		key := fileKey{
			name: normalizeNameForMatch(info.Name()),
			size: info.Size(),
		}
		files[key] = append(files[key], path)
		count++

		if onFile != nil {
			onFile(path, count)
		}

		return nil
	})

	return files, count, err
}

func findDuplicates(dir1Map, dir2Map map[fileKey][]string) []Duplicate {
	var duplicates []Duplicate

	for key, paths1 := range dir1Map {
		paths2, ok := dir2Map[key]
		if !ok {
			continue
		}

		for _, p1 := range paths1 {
			for _, p2 := range paths2 {
				if pathsEqual(p1, p2) {
					continue
				}
				duplicates = append(duplicates, Duplicate{
					FileName: key.name,
					Size:     key.size,
					Path1:    p1,
					Path2:    p2,
				})
			}
		}
	}

	return duplicates
}

func findDuplicatesWithin(files map[fileKey][]string) []Duplicate {
	var duplicates []Duplicate

	for key, paths := range files {
		if len(paths) < 2 {
			continue
		}

		for i := 0; i < len(paths); i++ {
			for j := i + 1; j < len(paths); j++ {
				p1 := paths[i]
				p2 := paths[j]
				if pathsEqual(p1, p2) {
					continue
				}
				duplicates = append(duplicates, Duplicate{
					FileName: key.name,
					Size:     key.size,
					Path1:    p1,
					Path2:    p2,
				})
			}
		}
	}

	return duplicates
}

func directoriesEqual(a, b string) (bool, error) {
	absA, err := filepath.Abs(filepath.Clean(a))
	if err != nil {
		return false, err
	}
	absB, err := filepath.Abs(filepath.Clean(b))
	if err != nil {
		return false, err
	}

	if runtime.GOOS == "windows" {
		return strings.EqualFold(absA, absB), nil
	}

	return absA == absB, nil
}

func pathsEqual(a, b string) bool {
	absA, errA := filepath.Abs(filepath.Clean(a))
	absB, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}

	if runtime.GOOS == "windows" {
		return strings.EqualFold(absA, absB)
	}

	return absA == absB
}

func normalizeNameForMatch(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	// Collapse common copy suffixes such as "(1)", "- Copy", "Copy (2)".
	for {
		trimmed := copySuffixPattern.ReplaceAllString(base, "")
		if trimmed == base {
			break
		}
		base = strings.TrimSpace(trimmed)
	}

	return strings.ToLower(base + ext)
}

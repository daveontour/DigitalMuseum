package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ValidateUnderRoots(path string, roots []string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	absPath = filepath.Clean(absPath)

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlinks are not supported")
	}

	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" {
			continue
		}

		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absRoot = filepath.Clean(absRoot)

		rel, err := filepath.Rel(absRoot, absPath)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return nil
	}

	return fmt.Errorf("path is outside the scanned directories")
}

func CleanRoots(roots []string) []string {
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root != "" {
			cleaned = append(cleaned, root)
		}
	}
	return cleaned
}

package delete

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/filedupe/paths"
)

type Result struct {
	Deleted []string          `json:"deleted"`
	Failed  map[string]string `json:"failed,omitempty"`
	Hint    string            `json:"hint,omitempty"`
}

func Files(allowedRoots []string, pathsToDelete []string) (Result, error) {
	roots := paths.CleanRoots(allowedRoots)
	if len(roots) == 0 {
		return Result{}, fmt.Errorf("at least one allowed directory root is required")
	}
	if len(pathsToDelete) == 0 {
		return Result{}, fmt.Errorf("no files selected")
	}

	result := Result{
		Failed: make(map[string]string),
	}

	seen := make(map[string]struct{}, len(pathsToDelete))
	for _, rawPath := range pathsToDelete {
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		if err := paths.ValidateUnderRoots(path, roots); err != nil {
			result.Failed[path] = err.Error()
			continue
		}

		info, err := os.Lstat(path)
		if err != nil {
			result.Failed[path] = err.Error()
			continue
		}
		if info.IsDir() {
			result.Failed[path] = "path is a directory"
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			result.Failed[path] = "symlinks are not supported"
			continue
		}

		if err := removeFile(path); err != nil {
			result.Failed[path] = describeRemoveError(err)
			continue
		}

		result.Deleted = append(result.Deleted, path)
	}

	if sawAccessDenied(result.Failed) {
		result.Hint = accessDeniedHint()
	}

	if len(result.Deleted) == 0 && len(result.Failed) > 0 {
		return Result{}, fmt.Errorf("failed to delete selected files")
	}

	return result, nil
}

package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FindAttachmentFile finds an attachment file by exact match or ending with filename.
// Returns the full path if found, or error if not found.
func FindAttachmentFile(baseDir, filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename is empty")
	}

	exactPath := filepath.Join(baseDir, filename)
	if _, err := os.Stat(exactPath); err == nil {
		return exactPath, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), filename) {
			return filepath.Join(baseDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("file not found: %s", filename)
}

// FindAttachmentFileWithFallback finds an attachment file with fallback logic.
// Handles .heic -> .jpg and .opus -> .mp3 fallbacks.
func FindAttachmentFileWithFallback(baseDir, filename string) (string, string, error) {
	path, err := FindAttachmentFile(baseDir, filename)
	if err == nil {
		return path, filename, nil
	}

	if strings.HasSuffix(strings.ToLower(filename), ".heic") {
		jpgFilename := filename[:len(filename)-5] + ".jpg"
		path, err := FindAttachmentFile(baseDir, jpgFilename)
		if err == nil {
			return path, jpgFilename, nil
		}
	}

	if strings.HasSuffix(strings.ToLower(filename), ".opus") {
		mp3Filename := filename[:len(filename)-5] + ".mp3"
		path, err := FindAttachmentFile(baseDir, mp3Filename)
		if err == nil {
			return path, mp3Filename, nil
		}
	}

	return "", filename, fmt.Errorf("file not found: %s", filename)
}

// AttachmentIndex holds a single directory listing for fast repeated attachment lookups
// (exact path, then suffix match, then HEIC/opus fallbacks) without re-reading the directory.
type AttachmentIndex struct {
	baseDir string
	names   []string // non-directory filenames in ReadDir order
}

// NewAttachmentIndex lists baseDir once and returns an index for FindWithFallback.
func NewAttachmentIndex(baseDir string) (*AttachmentIndex, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	return &AttachmentIndex{baseDir: baseDir, names: names}, nil
}

// newAttachmentIndexFromNames builds an index from an already-listed set of basenames
// (e.g. from a job-wide directory walk). baseDir should be absolute and clean.
func newAttachmentIndexFromNames(baseDir string, names []string) *AttachmentIndex {
	return &AttachmentIndex{baseDir: baseDir, names: names}
}

// BuildWhatsAppAttachmentIndices walks the WhatsApp job root once: each immediate
// subdirectory is a conversation folder; only files directly in that folder are indexed
// (same scope as FindAttachmentFile). Nested folders under a conversation are skipped.
// Keys are absolute, clean paths to each conversation directory.
func BuildWhatsAppAttachmentIndices(root string) (map[string]*AttachmentIndex, error) {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(rootAbs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", rootAbs)
	}

	filesByDir := make(map[string][]string)

	err = filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path == rootAbs {
				return nil
			}
			rel, err := filepath.Rel(rootAbs, path)
			if err != nil {
				return err
			}
			if strings.Contains(rel, string(filepath.Separator)) {
				return filepath.SkipDir
			}
			return nil
		}

		parentAbs, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return err
		}
		relParent, err := filepath.Rel(rootAbs, parentAbs)
		if err != nil {
			return err
		}
		if relParent == "." || strings.Contains(relParent, string(filepath.Separator)) {
			return nil
		}

		filesByDir[parentAbs] = append(filesByDir[parentAbs], filepath.Base(path))
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make(map[string]*AttachmentIndex, len(filesByDir))
	for dir, names := range filesByDir {
		namesCopy := append([]string(nil), names...)
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		out[absDir] = newAttachmentIndexFromNames(absDir, namesCopy)
	}
	return out, nil
}

// FindWithFallback matches FindAttachmentFileWithFallback semantics using the cached listing.
func (idx *AttachmentIndex) FindWithFallback(filename string) (path string, actualFilename string, err error) {
	if idx == nil {
		return "", filename, fmt.Errorf("attachment index is nil")
	}
	path, actualFilename, err = idx.find(filename)
	if err == nil {
		return path, actualFilename, nil
	}

	if strings.HasSuffix(strings.ToLower(filename), ".heic") {
		jpgFilename := filename[:len(filename)-5] + ".jpg"
		path, actualFilename, err = idx.find(jpgFilename)
		if err == nil {
			return path, actualFilename, nil
		}
	}

	if strings.HasSuffix(strings.ToLower(filename), ".opus") {
		mp3Filename := filename[:len(filename)-5] + ".mp3"
		path, actualFilename, err = idx.find(mp3Filename)
		if err == nil {
			return path, actualFilename, nil
		}
	}

	return "", filename, fmt.Errorf("file not found: %s", filename)
}

func (idx *AttachmentIndex) find(filename string) (string, string, error) {
	if filename == "" {
		return "", "", fmt.Errorf("filename is empty")
	}

	exactPath := filepath.Join(idx.baseDir, filename)
	if _, err := os.Stat(exactPath); err == nil {
		return exactPath, filename, nil
	}

	for _, name := range idx.names {
		if strings.HasSuffix(name, filename) {
			return filepath.Join(idx.baseDir, name), name, nil
		}
	}

	return "", filename, fmt.Errorf("file not found: %s", filename)
}

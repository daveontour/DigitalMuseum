package utils

import (
	"fmt"
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

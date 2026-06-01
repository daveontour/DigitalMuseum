package utils

import (
	"os"
	"time"
)

// FileCreationTime returns the filesystem creation/birth time when the OS exposes it,
// otherwise the file's modification time.
func FileCreationTime(path string) (time.Time, error) {
	if t, ok := fileBirthTime(path); ok && !t.IsZero() {
		return t, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

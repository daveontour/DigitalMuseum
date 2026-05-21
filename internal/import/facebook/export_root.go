package facebook

import (
	"os"
	"path/filepath"
	"strings"
)

const facebookMarker = "your_facebook_activity"

// DetectFacebookExportRoot searches upward from directoryPath for a directory
// containing "your_facebook_activity".
func DetectFacebookExportRoot(directoryPath, uri string) (string, bool) {
	if directoryPath == "" {
		return "", false
	}

	absDir, err := filepath.Abs(directoryPath)
	if err != nil {
		return "", false
	}

	current := absDir
	maxLevels := 10

	for i := 0; i < maxLevels; i++ {
		markerPath := filepath.Join(current, facebookMarker)
		if pathExists(markerPath, true) {
			if uri != "" {
				testPath := filepath.Join(current, uri)
				if pathExists(testPath, false) || pathExists(testPath, true) {
					return current, true
				}
			} else {
				return current, true
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	if uri != "" && strings.Contains(uri, facebookMarker) {
		parts := strings.SplitN(uri, facebookMarker, 2)
		if len(parts) > 0 && parts[0] != "" {
			uriPrefix := strings.TrimSuffix(strings.Trim(parts[0], "/"), "/")
			current := absDir
			for i := 0; i < maxLevels; i++ {
				var testMarker string
				if strings.HasPrefix(uriPrefix, "/") {
					testMarker = filepath.Join(strings.TrimPrefix(uriPrefix, "/"), facebookMarker)
				} else {
					testMarker = filepath.Join(current, uriPrefix, facebookMarker)
				}
				if pathExists(testMarker, true) {
					return filepath.Dir(testMarker), true
				}
				parent := filepath.Dir(current)
				if parent == current {
					break
				}
				current = parent
			}
		}
	}

	return "", false
}

func pathExists(path string, requireDir bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if requireDir {
		return info.IsDir()
	}
	return !info.IsDir()
}

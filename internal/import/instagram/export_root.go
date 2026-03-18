package instagram

import (
	"os"
	"path/filepath"
	"strings"
)

const instagramMarker = "your_instagram_activity"

// DetectInstagramExportRoot searches upward from directoryPath for a directory
// containing "your_instagram_activity".
func DetectInstagramExportRoot(directoryPath, uri string) (string, bool) {
	if directoryPath == "" {
		return "", false
	}

	absDir, err := filepath.Abs(directoryPath)
	if err != nil {
		return "", false
	}

	if root, ok := detectByMarker(absDir, uri); ok {
		return root, true
	}

	if uri != "" {
		variations := []string{uri}
		if strings.Contains(uri, "/inbox/") {
			variations = append(variations, strings.Replace(uri, "/inbox/", "/inboxtest/", 1))
		} else if strings.Contains(uri, "/inboxtest/") {
			variations = append(variations, strings.Replace(uri, "/inboxtest/", "/inbox/", 1))
		}
		for _, u := range variations {
			if root, ok := detectByMarker(absDir, u); ok {
				return root, true
			}
		}
	}

	return "", false
}

func detectByMarker(absDir, uri string) (string, bool) {
	current := absDir
	maxLevels := 10

	for i := 0; i < maxLevels; i++ {
		markerPath := filepath.Join(current, instagramMarker)
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

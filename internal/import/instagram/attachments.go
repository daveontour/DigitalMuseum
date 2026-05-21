package instagram

import (
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/import/utils"
)

// FindPhotoFile locates a photo file by URI
func FindPhotoFile(conversationDir, uri, exportRoot string) (string, bool) {
	if uri == "" {
		return "", false
	}

	filename := filepath.Base(uri)

	if exportRoot != "" {
		exportPath := filepath.Join(exportRoot, uri)
		if pathExists(exportPath, false) {
			return exportPath, true
		}
		altURI := uri
		if strings.Contains(uri, "/inbox/") {
			altURI = strings.Replace(uri, "/inbox/", "/inboxtest/", 1)
		} else if strings.Contains(uri, "/inboxtest/") {
			altURI = strings.Replace(uri, "/inboxtest/", "/inbox/", 1)
		}
		if altURI != uri {
			altPath := filepath.Join(exportRoot, altURI)
			if pathExists(altPath, false) {
				return altPath, true
			}
		}
	}

	photosPath := filepath.Join(conversationDir, "photos", filename)
	if pathExists(photosPath, false) {
		return photosPath, true
	}

	convPath := filepath.Join(conversationDir, filename)
	if pathExists(convPath, false) {
		return convPath, true
	}

	return "", false
}

// GuessMIMEType returns MIME type from filename
func GuessMIMEType(filename string) string {
	mt := utils.DetectMIMEType(filename)
	if mt != "" {
		return mt
	}
	return "image/jpeg"
}

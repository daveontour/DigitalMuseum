package importstorage

import (
	"path/filepath"
	"strings"
)

// NormalizeFilesystemUploadMediaType picks a stable image/* MIME for multipart uploads when
// the browser sends a generic type (often application/octet-stream for HEIC/HEIF).
func NormalizeFilesystemUploadMediaType(sourceRef string, headerType string, data []byte) string {
	t := strings.TrimSpace(headerType)
	lt := strings.ToLower(t)
	if strings.HasPrefix(lt, "image/") {
		return t
	}

	ext := strings.ToLower(filepath.Ext(sourceRef))
	switch ext {
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	}

	if isGenericOrEmptyMIME(lt) && len(data) >= 12 && string(data[4:8]) == "ftyp" {
		brand := string(data[8:12])
		switch brand {
		case "heic", "heix", "hevc", "heim", "heis":
			return "image/heic"
		case "mif1", "msf1":
			return "image/heif"
		}
	}

	if t == "" {
		return "application/octet-stream"
	}
	return t
}

func isGenericOrEmptyMIME(lt string) bool {
	if lt == "" {
		return true
	}
	return lt == "application/octet-stream" || lt == "binary/octet-stream"
}

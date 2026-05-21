package imessage

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// 1×1 transparent PNG (minimal).
const imazingPlaceholderPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

var imazingPlaceholderPNG = mustDecodeB64(imazingPlaceholderPNGBase64)

func mustDecodeB64(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic("imessage: invalid embedded placeholder PNG: " + err.Error())
	}
	return b
}

// ImazingPlaceholderPNG returns the dummy image bytes used for generic Attachment.* CSV rows.
func ImazingPlaceholderPNG() []byte {
	return imazingPlaceholderPNG
}

var genericAttachmentNameRegexp = regexp.MustCompile(`(?i)^attachment\.`)

// IsGenericImazingAttachmentName reports whether the attachment basename is iMazing's generic
// "Attachment.<ext>" form (always replaced with a placeholder per import rules).
func IsGenericImazingAttachmentName(filename string) bool {
	base := filepath.Base(strings.TrimSpace(filename))
	if base == "" || base == "." {
		return false
	}
	return genericAttachmentNameRegexp.MatchString(base)
}

var imageExtOK = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {}, ".webp": {},
	".heic": {}, ".heif": {}, ".tiff": {}, ".tif": {}, ".bmp": {},
}

// ListImageBasenames returns sorted basenames of image files in dir (non-recursive).
func ListImageBasenames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if _, ok := imageExtOK[ext]; ok {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

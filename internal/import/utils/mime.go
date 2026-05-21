package utils

import (
	"mime"
	"path/filepath"
	"strings"
)

// DetectMIMEType detects MIME type from filename or file path.
func DetectMIMEType(filename string) string {
	if filename == "" {
		return "application/octet-stream"
	}

	ext := filepath.Ext(filename)
	if ext != "" {
		mimeType := mime.TypeByExtension(ext)
		if mimeType != "" {
			return mimeType
		}
	}

	baseName := strings.ToLower(filename)
	if strings.Contains(baseName, ".jpg") || strings.Contains(baseName, ".jpeg") {
		return "image/jpeg"
	}
	if strings.Contains(baseName, ".png") {
		return "image/png"
	}
	if strings.Contains(baseName, ".gif") {
		return "image/gif"
	}
	if strings.Contains(baseName, ".mp4") {
		return "video/mp4"
	}
	if strings.Contains(baseName, ".mp3") {
		return "audio/mpeg"
	}
	if strings.Contains(baseName, ".opus") {
		return "audio/opus"
	}
	if strings.Contains(baseName, ".heic") {
		return "image/heic"
	}

	return "application/octet-stream"
}

// NormalizeMIMEType normalizes generic MIME types to specific ones.
// Handles cases like "Image", "Video", "Audio", "Attachment" from CSV.
func NormalizeMIMEType(mimeType, filename string) string {
	if mimeType == "" || mimeType == "Image" || mimeType == "Video" || mimeType == "Audio" || mimeType == "Attachment" {
		return DetectMIMEType(filename)
	}
	return mimeType
}

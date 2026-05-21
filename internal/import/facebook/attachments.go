package facebook

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/import/utils"
)

var errFound = errors.New("found")

// AdditionalAttachment represents extra attachments beyond the first
type AdditionalAttachment struct {
	Filename string
	Type     string
	Data     []byte
}

// FindAttachmentFile locates an attachment file by URI
func FindAttachmentFile(conversationDir, uri, exportRoot string) (string, bool) {
	if uri == "" {
		return "", false
	}

	relPath := filepath.Join(conversationDir, uri)
	if pathExists(relPath, false) {
		return relPath, true
	}

	filename := filepath.Base(uri)
	filenamePath := filepath.Join(conversationDir, filename)
	if pathExists(filenamePath, false) {
		return filenamePath, true
	}

	for _, subdir := range []string{"photos", "videos", "files"} {
		subPath := filepath.Join(conversationDir, subdir, filename)
		if pathExists(subPath, false) {
			return subPath, true
		}
	}

	if exportRoot != "" {
		exportPath := filepath.Join(exportRoot, uri)
		if pathExists(exportPath, false) {
			return exportPath, true
		}
	}

	var foundPath string
	filepath.WalkDir(conversationDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == filename {
			foundPath = p
			return errFound
		}
		return nil
	})
	if foundPath != "" {
		return foundPath, true
	}

	return "", false
}

// GuessMIMEType returns MIME type from filename
func GuessMIMEType(filename string) string {
	mt := utils.DetectMIMEType(filename)
	if mt != "" {
		return mt
	}
	return "application/octet-stream"
}

// GetFirstAttachment extracts the primary attachment from a message
func GetFirstAttachment(msg FacebookMessage, conversationDir, exportRoot string) (string, string, []byte, []AdditionalAttachment) {
	var primaryFilename, primaryType string
	var primaryData []byte
	var additional []AdditionalAttachment

	photos := msg.Photos
	videos := msg.Videos
	files := msg.Files

	if len(photos) > 0 {
		primaryFilename, primaryType, primaryData = readAttachment(photos[0].URI, conversationDir, exportRoot, "image/jpeg")
		for i := 1; i < len(photos); i++ {
			fn, mt, data := readAttachment(photos[i].URI, conversationDir, exportRoot, "image/jpeg")
			if fn != "" {
				additional = append(additional, AdditionalAttachment{Filename: fn, Type: mt, Data: data})
			}
		}
	}

	if primaryData == nil && len(photos) > 0 {
		uri := photos[0].URI
		if strings.HasSuffix(strings.ToLower(uri), ".heic") {
			jpgURI := uri[:len(uri)-5] + ".jpg"
			primaryFilename, primaryType, primaryData = readAttachment(jpgURI, conversationDir, exportRoot, "image/jpeg")
		}
	}

	if primaryData == nil && len(videos) > 0 {
		primaryFilename, primaryType, primaryData = readAttachment(videos[0].URI, conversationDir, exportRoot, "video/mp4")
		for i := 1; i < len(videos); i++ {
			fn, mt, data := readAttachment(videos[i].URI, conversationDir, exportRoot, "video/mp4")
			if fn != "" {
				additional = append(additional, AdditionalAttachment{Filename: fn, Type: mt, Data: data})
			}
		}
	}

	if primaryData == nil && len(files) > 0 {
		primaryFilename, primaryType, primaryData = readAttachmentWithOpusFallback(files[0].URI, conversationDir, exportRoot)
		if primaryType == "" {
			primaryType = "application/octet-stream"
		}
		for i := 1; i < len(files); i++ {
			fn, mt, data := readAttachmentWithOpusFallback(files[i].URI, conversationDir, exportRoot)
			if fn != "" {
				additional = append(additional, AdditionalAttachment{Filename: fn, Type: mt, Data: data})
			}
		}
	}

	if primaryData != nil && len(photos) > 0 {
		for _, v := range videos {
			fn, mt, data := readAttachment(v.URI, conversationDir, exportRoot, "video/mp4")
			if fn != "" {
				additional = append(additional, AdditionalAttachment{Filename: fn, Type: mt, Data: data})
			}
		}
		for _, f := range files {
			fn, mt, data := readAttachmentWithOpusFallback(f.URI, conversationDir, exportRoot)
			if fn != "" {
				additional = append(additional, AdditionalAttachment{Filename: fn, Type: mt, Data: data})
			}
		}
	} else if primaryData != nil && len(videos) > 0 {
		for _, f := range files {
			fn, mt, data := readAttachmentWithOpusFallback(f.URI, conversationDir, exportRoot)
			if fn != "" {
				additional = append(additional, AdditionalAttachment{Filename: fn, Type: mt, Data: data})
			}
		}
	}

	return primaryFilename, primaryType, primaryData, additional
}

func readAttachment(uri, conversationDir, exportRoot, defaultType string) (string, string, []byte) {
	path, ok := FindAttachmentFile(conversationDir, uri, exportRoot)
	if !ok {
		return filepath.Base(uri), "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return filepath.Base(uri), "", nil
	}
	filename := filepath.Base(uri)
	mt := GuessMIMEType(filename)
	if mt == "application/octet-stream" {
		mt = defaultType
	}
	return filename, mt, data
}

func readAttachmentWithOpusFallback(uri, conversationDir, exportRoot string) (string, string, []byte) {
	path, ok := FindAttachmentFile(conversationDir, uri, exportRoot)
	if !ok && strings.HasSuffix(strings.ToLower(uri), ".opus") {
		mp3URI := uri[:len(uri)-5] + ".mp3"
		path, ok = FindAttachmentFile(conversationDir, mp3URI, exportRoot)
		if ok {
			uri = mp3URI
		}
	}
	if !ok {
		return filepath.Base(uri), "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return filepath.Base(uri), "", nil
	}
	filename := filepath.Base(uri)
	return filename, GuessMIMEType(filename), data
}

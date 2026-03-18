package facebookalbums

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/daveontour/digitalmuseum/internal/import/facebook"
	"github.com/daveontour/digitalmuseum/internal/import/utils"
	"github.com/daveontour/digitalmuseum/internal/importstorage"
	"github.com/jackc/pgx/v5/pgxpool"
)

const albumImageBatchSize = 25

var errFound = errors.New("found")

// AlbumExport represents the structure of a Facebook album JSON file.
type AlbumExport struct {
	Name                  string           `json:"name"`
	Description           string           `json:"description"`
	CoverPhoto            *AlbumCoverPhoto `json:"cover_photo"`
	LastModifiedTimestamp *int64           `json:"last_modified_timestamp"`
	Photos                []AlbumPhoto     `json:"photos"`
}

// AlbumCoverPhoto represents the cover photo of an album.
type AlbumCoverPhoto struct {
	URI string `json:"uri"`
}

// AlbumPhoto represents a photo in an album.
type AlbumPhoto struct {
	URI               string `json:"uri"`
	CreationTimestamp *int64 `json:"creation_timestamp"`
	Title             string `json:"title"`
	Description       string `json:"description"`
}

// ImportStats holds statistics about the import process.
type ImportStats struct {
	AlbumsProcessed       int
	TotalAlbums           int
	AlbumsImported        int
	ImagesImported        int
	ImagesFound           int
	ImagesMissing         int
	MissingImageFilenames []string
	missingImageSet       map[string]struct{}
	Errors                int
	CurrentAlbum          string
	mu                    sync.Mutex
}

func (s *ImportStats) copyStats() ImportStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ImportStats{
		AlbumsProcessed:       s.AlbumsProcessed,
		TotalAlbums:           s.TotalAlbums,
		AlbumsImported:        s.AlbumsImported,
		ImagesImported:        s.ImagesImported,
		ImagesFound:           s.ImagesFound,
		ImagesMissing:         s.ImagesMissing,
		MissingImageFilenames: append([]string(nil), s.MissingImageFilenames...),
		Errors:                s.Errors,
		CurrentAlbum:          s.CurrentAlbum,
	}
}

// ProgressCallback is called after each album is processed.
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if the import should be cancelled.
type CancelledCheck func() bool

// ImportFacebookAlbumsFromDirectory imports Facebook albums from a directory structure.
func ImportFacebookAlbumsFromDirectory(
	ctx context.Context,
	pool *pgxpool.Pool,
	directoryPath string,
	progressCallback ProgressCallback,
	cancelledCheck CancelledCheck,
	exportRootOverride string,
) (*ImportStats, error) {
	dirInfo, err := os.Stat(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("directory does not exist or is not accessible: %w", err)
	}
	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", directoryPath)
	}

	albumDirs, err := findAlbumDirsRecursive(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	var jsonFiles []string
	for _, files := range albumDirs {
		jsonFiles = append(jsonFiles, files...)
	}
	sort.Strings(jsonFiles)

	stats := &ImportStats{
		TotalAlbums:           len(jsonFiles),
		MissingImageFilenames: []string{},
		missingImageSet:       make(map[string]struct{}),
	}

	exportRoot := exportRootOverride
	if exportRoot == "" {
		if root, ok := facebook.DetectFacebookExportRoot(directoryPath, ""); ok {
			exportRoot = root
		}
	}

	searchDirs := []string{directoryPath}
	if exportRoot != "" && exportRoot != directoryPath {
		searchDirs = append(searchDirs, exportRoot)
	}
	filenameCache := BuildFilenameCache(searchDirs)

	storage := importstorage.NewFacebookAlbumStorage(pool)

	numWorkers := runtime.NumCPU()
	if numWorkers > len(jsonFiles) {
		numWorkers = len(jsonFiles)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	type albumWork struct {
		jsonFile string
		albumDir string
	}
	workChan := make(chan albumWork, len(jsonFiles))
	for albumDir, files := range albumDirs {
		for _, jf := range files {
			workChan <- albumWork{jsonFile: jf, albumDir: albumDir}
		}
	}
	close(workChan)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range workChan {
				if cancelledCheck != nil && cancelledCheck() {
					return
				}
				select {
				case <-ctx.Done():
					return
				default:
				}

				stats.mu.Lock()
				stats.AlbumsProcessed++
				stats.CurrentAlbum = filepath.Base(work.jsonFile)
				stats.mu.Unlock()

				err := processAlbumJSONFile(ctx, storage, work.jsonFile, work.albumDir, stats, exportRoot, filenameCache, cancelledCheck)
				if err != nil {
					stats.mu.Lock()
					stats.Errors++
					stats.mu.Unlock()
				}

				if progressCallback != nil {
					progressCallback(stats.copyStats())
				}
			}
		}()
	}
	wg.Wait()

	return stats, nil
}

func findAlbumDirsRecursive(directoryPath string) (map[string][]string, error) {
	var albumDirPaths []string
	err := filepath.WalkDir(directoryPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if !strings.Contains(strings.ToLower(d.Name()), "album") {
			return nil
		}
		albumDirPaths = append(albumDirPaths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	albumDirs := make(map[string][]string)
	for _, dirPath := range albumDirPaths {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		var jsonFiles []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".json") {
				jsonFiles = append(jsonFiles, filepath.Join(dirPath, e.Name()))
			}
		}
		if len(jsonFiles) > 0 {
			albumDirs[dirPath] = jsonFiles
		}
	}
	return albumDirs, nil
}

func processAlbumJSONFile(ctx context.Context, storage *importstorage.FacebookAlbumStorage, jsonFilePath, albumDir string, stats *ImportStats, exportRoot string, filenameCache map[string]string, cancelledCheck CancelledCheck) error {
	data, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return fmt.Errorf("failed to read JSON: %w", err)
	}

	export, err := parseAlbumJSON(data)
	if err != nil {
		return err
	}

	albumName := export.Name
	if albumName == "" {
		albumName = strings.TrimSuffix(filepath.Base(jsonFilePath), filepath.Ext(jsonFilePath))
	}

	photosWithURI := 0
	for _, p := range export.Photos {
		if p.URI != "" {
			photosWithURI++
		}
	}
	if photosWithURI == 0 {
		return nil
	}

	var coverPhotoURI string
	if export.CoverPhoto != nil {
		coverPhotoURI = export.CoverPhoto.URI
	}

	albumID, wasCreated, err := storage.SaveOrUpdateAlbum(ctx, albumName, export.Description, coverPhotoURI, parseTimestampMs(export.LastModifiedTimestamp))
	if err != nil {
		return fmt.Errorf("failed to save album: %w", err)
	}

	if wasCreated {
		stats.mu.Lock()
		stats.AlbumsImported++
		stats.mu.Unlock()
	}

	var batch []importstorage.BatchAlbumImageItem
	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		n, err := storage.SaveAlbumImagesBatch(ctx, batch)
		if err != nil {
			stats.mu.Lock()
			stats.Errors += len(batch)
			stats.mu.Unlock()
		} else {
			stats.mu.Lock()
			stats.ImagesImported += n
			stats.mu.Unlock()
		}
		batch = batch[:0]
	}

	for _, photo := range export.Photos {
		if cancelledCheck != nil && cancelledCheck() {
			flushBatch()
			return context.Canceled
		}
		select {
		case <-ctx.Done():
			flushBatch()
			return ctx.Err()
		default:
		}

		uri := photo.URI
		if uri == "" {
			continue
		}

		imagePath, ok := FindImageFile(albumDir, uri, exportRoot, filenameCache)
		if !ok {
			if jpgURI := TryHEICFallback(uri); jpgURI != "" {
				imagePath, ok = FindImageFile(albumDir, jpgURI, exportRoot, filenameCache)
				if ok {
					uri = jpgURI
				}
			}
		}
		if !ok {
			if mp3URI := TryOpusFallback(uri); mp3URI != "" {
				imagePath, ok = FindImageFile(albumDir, mp3URI, exportRoot, filenameCache)
				if ok {
					uri = mp3URI
				}
			}
		}

		var filename, imageType string
		var imageData []byte

		if ok {
			filename, imageType, imageData = ReadImageFile(imagePath, uri)
			if len(imageData) > 0 {
				stats.mu.Lock()
				stats.ImagesFound++
				stats.mu.Unlock()
			} else {
				stats.mu.Lock()
				stats.ImagesMissing++
				missingKey := albumName + "/" + filepath.Base(uri)
				if _, exists := stats.missingImageSet[missingKey]; !exists {
					stats.missingImageSet[missingKey] = struct{}{}
					stats.MissingImageFilenames = append(stats.MissingImageFilenames, missingKey)
				}
				stats.mu.Unlock()
			}
		} else {
			stats.mu.Lock()
			stats.ImagesMissing++
			missingKey := albumName + "/" + filepath.Base(uri)
			if _, exists := stats.missingImageSet[missingKey]; !exists {
				stats.missingImageSet[missingKey] = struct{}{}
				stats.MissingImageFilenames = append(stats.MissingImageFilenames, missingKey)
			}
			stats.mu.Unlock()
		}

		creationTs := parseTimestampMs(photo.CreationTimestamp)
		title := photo.Title
		description := photo.Description
		if filename == "" {
			filename = filepath.Base(uri)
		}

		batch = append(batch, importstorage.BatchAlbumImageItem{
			AlbumID:           albumID,
			URI:               uri,
			Filename:          filename,
			CreationTimestamp: creationTs,
			Title:             title,
			Description:       description,
			ImageData:         imageData,
			ImageType:         imageType,
			AlbumName:         albumName,
		})

		if len(batch) >= albumImageBatchSize {
			flushBatch()
		}
	}

	flushBatch()
	return nil
}

func parseTimestampMs(ts *int64) *time.Time {
	if ts == nil {
		return nil
	}
	t := time.UnixMilli(*ts)
	return &t
}

func parseAlbumJSON(data []byte) (*AlbumExport, error) {
	var export AlbumExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return &export, nil
}

func guessMIMEType(filename string) string {
	mt := utils.DetectMIMEType(filename)
	if mt != "" {
		return mt
	}
	return "image/jpeg"
}

// FindImageFile locates an image file by URI.
func FindImageFile(albumDir, uri, exportRoot string, filenameCache map[string]string) (string, bool) {
	if uri == "" {
		return "", false
	}
	if exportRoot != "" {
		p := filepath.Join(exportRoot, uri)
		if pathExists(p, false) {
			return p, true
		}
	}
	relPath := filepath.Join(albumDir, uri)
	if pathExists(relPath, false) {
		return relPath, true
	}
	filename := filepath.Base(uri)
	filenamePath := filepath.Join(albumDir, filename)
	if pathExists(filenamePath, false) {
		return filenamePath, true
	}
	if filenameCache != nil {
		if p, ok := filenameCache[filename]; ok {
			return p, true
		}
		return "", false
	}
	var found string
	filepath.WalkDir(albumDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == filename {
			found = p
			return errFound
		}
		return nil
	})
	if found != "" {
		return found, true
	}
	return "", false
}

// BuildFilenameCache walks rootDirs and builds a filename->path map for O(1) lookups.
func BuildFilenameCache(rootDirs []string) map[string]string {
	cache := make(map[string]string)
	for _, rootDir := range rootDirs {
		if rootDir == "" {
			continue
		}
		filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				name := d.Name()
				if _, exists := cache[name]; !exists {
					cache[name] = p
				}
			}
			return nil
		})
	}
	return cache
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

// ReadImageFile reads image data from path.
func ReadImageFile(imagePath, uri string) (filename, mimeType string, data []byte) {
	d, err := os.ReadFile(imagePath)
	if err != nil {
		return filepath.Base(uri), "", nil
	}
	fn := filepath.Base(uri)
	mt := guessMIMEType(fn)
	if mt == "" {
		mt = "image/jpeg"
	}
	return fn, mt, d
}

// TryHEICFallback tries .heic -> .jpg if original not found.
func TryHEICFallback(uri string) string {
	if strings.HasSuffix(strings.ToLower(uri), ".heic") {
		return uri[:len(uri)-5] + ".jpg"
	}
	return ""
}

// TryOpusFallback tries .opus -> .mp3.
func TryOpusFallback(uri string) string {
	if strings.HasSuffix(strings.ToLower(uri), ".opus") {
		return uri[:len(uri)-5] + ".mp3"
	}
	return ""
}

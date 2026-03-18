package filesystem

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/daveontour/digitalmuseum/internal/import/utils"
	"github.com/daveontour/digitalmuseum/internal/importstorage"
)

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".bmp": true, ".tiff": true, ".tif": true, ".webp": true,
	".heic": true, ".heif": true, ".avif": true, ".ico": true,
}

func shouldSkipPath(path string, name string) bool {
	if strings.Contains(path, ".photostructure") {
		return true
	}
	if strings.HasPrefix(name, "._") {
		return true
	}
	switch strings.ToLower(name) {
	case "thumbs.db", "desktop.ini", "ehthumbs.db", "ehthumbs.db-shm":
		return true
	}
	return false
}

func shouldExcludeDirectory(dirPath string, dirName string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		matched, _ := filepath.Match(pattern, dirName)
		if matched {
			return true
		}
		matched, _ = filepath.Match(pattern, dirPath)
		if matched {
			return true
		}
		if strings.Contains(dirPath, pattern) || strings.Contains(dirName, pattern) {
			return true
		}
	}
	return false
}

// ImportStats holds statistics about the import process
type ImportStats struct {
	TotalFiles       int
	FilesProcessed   int
	ImagesImported   int
	ImagesUpdated    int
	ImagesReferenced int
	Errors           int
	ErrorMessages    []string
	CurrentFile      string
	mu               sync.Mutex
}

func (s *ImportStats) copyStats() ImportStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ImportStats{
		TotalFiles:       s.TotalFiles,
		FilesProcessed:   s.FilesProcessed,
		ImagesImported:   s.ImagesImported,
		ImagesUpdated:    s.ImagesUpdated,
		ImagesReferenced: s.ImagesReferenced,
		Errors:           s.Errors,
		ErrorMessages:    append([]string(nil), s.ErrorMessages...),
		CurrentFile:      s.CurrentFile,
	}
}

// ProgressCallback is called after each image is processed
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if the import should be cancelled
type CancelledCheck func() bool

const progressCallbackInterval = 25
const imageBatchSize = 100

type imageWork struct {
	Path     string
	RootPath string
	Name     string
}

// ImportImagesFromDirectories imports images from one or more directory trees
func ImportImagesFromDirectories(
	ctx context.Context,
	storage *importstorage.ImageStorage,
	directories []string,
	excludePatterns []string,
	maxImages *int,
	referenceMode bool,
	progressCallback ProgressCallback,
	cancelledCheck CancelledCheck,
) (*ImportStats, error) {
	stats := &ImportStats{
		ErrorMessages: []string{},
	}

	var workItems []imageWork
	for _, rootDir := range directories {
		if maxImages != nil && len(workItems) >= *maxImages {
			break
		}

		rootPath, err := filepath.Abs(rootDir)
		if err != nil {
			return nil, fmt.Errorf("invalid path %s: %w", rootDir, err)
		}
		info, err := os.Stat(rootPath)
		if err != nil {
			return nil, fmt.Errorf("directory does not exist or is not accessible: %s: %w", rootPath, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("path is not a directory: %s", rootPath)
		}

		filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
			if cancelledCheck != nil && cancelledCheck() {
				return fmt.Errorf("cancelled")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err != nil {
				return nil
			}
			if d.IsDir() {
				if shouldExcludeDirectory(path, d.Name(), excludePatterns) {
					return filepath.SkipDir
				}
				return nil
			}
			if shouldSkipPath(path, d.Name()) {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if !imageExtensions[ext] {
				return nil
			}

			workItems = append(workItems, imageWork{Path: path, RootPath: rootPath, Name: d.Name()})
			stats.TotalFiles++

			if maxImages != nil && len(workItems) >= *maxImages {
				return filepath.SkipAll
			}
			return nil
		})
	}

	if len(workItems) == 0 {
		return stats, nil
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > len(workItems) {
		numWorkers = len(workItems)
	}

	workChan := make(chan imageWork, len(workItems))
	var wg sync.WaitGroup

	numWorkers = 4

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var batch []importstorage.BatchImageItem
			flushBatch := func() {
				if len(batch) == 0 {
					return
				}
				imp, upd, err := storage.SaveImagesBatch(ctx, batch)
				if err != nil {
					for _, item := range batch {
						_, isUpdate, saveErr := storage.SaveImage(ctx, item.SourceRef, item.ImageData, item.MediaType, item.Title, item.Tags, item.IsReferenced)
						if saveErr != nil {
							stats.mu.Lock()
							stats.Errors++
							stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("Error processing %s: %v", item.SourceRef, saveErr))
							stats.mu.Unlock()
						} else {
							stats.mu.Lock()
							if isUpdate {
								stats.ImagesUpdated++
							} else if item.IsReferenced {
								stats.ImagesReferenced++
							} else {
								stats.ImagesImported++
							}
							stats.mu.Unlock()
						}
					}
				} else {
					stats.mu.Lock()
					stats.ImagesImported += imp
					stats.ImagesUpdated += upd
					stats.mu.Unlock()
				}
				batch = batch[:0]
			}

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
				stats.FilesProcessed++
				stats.CurrentFile = work.Path
				stats.mu.Unlock()

				absPath, _ := filepath.Abs(work.Path)
				mediaType := utils.DetectMIMEType(work.Name)
				title := strings.TrimSuffix(work.Name, filepath.Ext(work.Name))
				tags := generateDirectoryTags(work.Path, work.RootPath)

				var imageData []byte
				if !referenceMode {
					var err error
					imageData, err = os.ReadFile(work.Path)
					if err != nil {
						stats.mu.Lock()
						stats.Errors++
						stats.ErrorMessages = append(stats.ErrorMessages, fmt.Sprintf("Error reading %s: %v", work.Path, err))
						stats.mu.Unlock()
						if progressCallback != nil {
							progressCallback(stats.copyStats())
						}
						continue
					}
				}

				batch = append(batch, importstorage.BatchImageItem{
					SourceRef:    absPath,
					ImageData:    imageData,
					MediaType:    mediaType,
					Title:        title,
					Tags:         tags,
					IsReferenced: referenceMode,
				})

				if len(batch) >= imageBatchSize {
					flushBatch()
					stats.mu.Lock()
					current := stats.FilesProcessed
					stats.mu.Unlock()
					if progressCallback != nil && (current%progressCallbackInterval == 0) {
						progressCallback(stats.copyStats())
					}
				}
			}
			flushBatch()
		}()
	}

	for _, work := range workItems {
		workChan <- work
	}
	close(workChan)
	wg.Wait()

	if progressCallback != nil {
		progressCallback(stats.copyStats())
	}

	return stats, nil
}

func generateDirectoryTags(filePath, rootPath string) string {
	rel, err := filepath.Rel(rootPath, filepath.Dir(filePath))
	if err != nil {
		return ""
	}
	if rel == "." || rel == ".." {
		return ""
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), ",")
}

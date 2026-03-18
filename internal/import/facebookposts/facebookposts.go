package facebookposts

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/daveontour/digitalmuseum/internal/import/facebook"
	"github.com/daveontour/digitalmuseum/internal/import/facebookalbums"
	"github.com/daveontour/digitalmuseum/internal/importstorage"
	"github.com/jackc/pgx/v5/pgxpool"
)

const postImageBatchSize = 25

var postsFilePattern = regexp.MustCompile(`your_posts__check_ins__photos_and_videos_\d+\.json`)

// PostsExport is the top-level array of posts in the JSON file.
type PostsExport []PostEntry

// PostEntry represents a single Facebook post.
type PostEntry struct {
	Timestamp   *int64           `json:"timestamp"`
	Title       string           `json:"title"`
	Data        []PostDataItem   `json:"data"`
	Attachments []PostAttachment `json:"attachments"`
}

// PostDataItem holds the text content of a post.
type PostDataItem struct {
	Post            string `json:"post"`
	UpdateTimestamp *int64 `json:"update_timestamp"`
}

// PostAttachment wraps an array of attachment data items.
type PostAttachment struct {
	Data []PostAttachmentData `json:"data"`
}

// PostAttachmentData can be a media item or an external link.
type PostAttachmentData struct {
	Media           *PostMediaItem       `json:"media"`
	ExternalContext *PostExternalContext `json:"external_context"`
}

// PostMediaItem represents a photo or video attached to a post.
type PostMediaItem struct {
	URI               string `json:"uri"`
	CreationTimestamp *int64 `json:"creation_timestamp"`
	Title             string `json:"title"`
	Description       string `json:"description"`
}

// PostExternalContext holds an external URL shared in a post.
type PostExternalContext struct {
	URL string `json:"url"`
}

// ImportStats holds statistics about the import process.
type ImportStats struct {
	PostsProcessed int
	TotalPosts     int
	PostsImported  int
	PostsUpdated   int
	WithMedia      int
	ImagesImported int
	ImagesFound    int
	ImagesMissing  int
	Errors         int
	CurrentPost    string
	mu             sync.Mutex
}

func (s *ImportStats) copyStats() ImportStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ImportStats{
		PostsProcessed: s.PostsProcessed,
		TotalPosts:     s.TotalPosts,
		PostsImported:  s.PostsImported,
		PostsUpdated:   s.PostsUpdated,
		WithMedia:      s.WithMedia,
		ImagesImported: s.ImagesImported,
		ImagesFound:    s.ImagesFound,
		ImagesMissing:  s.ImagesMissing,
		Errors:         s.Errors,
		CurrentPost:    s.CurrentPost,
	}
}

// ProgressCallback is called after each post is processed.
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if the import should be cancelled.
type CancelledCheck func() bool

// ImportFacebookPostsFromPath imports Facebook posts from a JSON file or directory.
func ImportFacebookPostsFromPath(
	ctx context.Context,
	pool *pgxpool.Pool,
	path string,
	exportRootOverride string,
	progressCallback ProgressCallback,
	cancelledCheck CancelledCheck,
) (*ImportStats, error) {
	jsonFiles, err := collectPostsFiles(path)
	if err != nil {
		return nil, err
	}
	if len(jsonFiles) == 0 {
		return nil, fmt.Errorf("no posts JSON files found at path: %s", path)
	}

	exportRoot := exportRootOverride
	if exportRoot == "" {
		if root, ok := facebook.DetectFacebookExportRoot(path, ""); ok {
			exportRoot = root
		}
	}

	searchDirs := []string{path}
	if exportRoot != "" && exportRoot != path {
		searchDirs = append(searchDirs, exportRoot)
	}
	filenameCache := facebookalbums.BuildFilenameCache(searchDirs)

	storage := importstorage.NewFacebookPostStorage(pool)

	var totalPosts int
	allParsed := make([]PostsExport, len(jsonFiles))
	for i, jf := range jsonFiles {
		data, err := os.ReadFile(jf)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", jf, err)
		}
		posts, err := parsePostsJSON(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", jf, err)
		}
		allParsed[i] = posts
		totalPosts += len(posts)
	}

	stats := &ImportStats{TotalPosts: totalPosts}

	for _, posts := range allParsed {
		for _, post := range posts {
			if cancelledCheck != nil && cancelledCheck() {
				return stats, context.Canceled
			}
			select {
			case <-ctx.Done():
				return stats, ctx.Err()
			default:
			}

			if err := processPost(ctx, storage, post, exportRoot, filenameCache, stats, cancelledCheck); err != nil {
				stats.mu.Lock()
				stats.Errors++
				stats.mu.Unlock()
			}

			if progressCallback != nil {
				progressCallback(stats.copyStats())
			}
		}
	}

	return stats, nil
}

func collectPostsFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not accessible: %w", err)
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if postsFilePattern.MatchString(d.Name()) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func processPost(ctx context.Context, storage *importstorage.FacebookPostStorage, entry PostEntry, exportRoot string, filenameCache map[string]string, stats *ImportStats, cancelledCheck CancelledCheck) error {
	ts := parseTimestampSec(entry.Timestamp)
	postText := extractPostText(entry)
	externalURL := extractExternalURL(entry)
	postType := determinePostType(entry)
	mediaItems := extractMediaItems(entry)

	albumDir := exportRoot

	titlePreview := entry.Title
	if len(titlePreview) > 50 {
		titlePreview = titlePreview[:50] + "..."
	}

	stats.mu.Lock()
	stats.PostsProcessed++
	stats.CurrentPost = titlePreview
	stats.mu.Unlock()

	postID, wasCreated, err := storage.SaveOrUpdatePost(ctx, ts, entry.Title, postText, externalURL, postType)
	if err != nil {
		return fmt.Errorf("failed to save post: %w", err)
	}

	stats.mu.Lock()
	if wasCreated {
		stats.PostsImported++
	} else {
		stats.PostsUpdated++
	}
	if len(mediaItems) > 0 {
		stats.WithMedia++
	}
	stats.mu.Unlock()

	if len(mediaItems) == 0 {
		return nil
	}

	var batch []importstorage.BatchPostImageItem
	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		n, err := storage.SavePostImagesBatch(ctx, batch)
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

	for _, mi := range mediaItems {
		if cancelledCheck != nil && cancelledCheck() {
			flushBatch()
			return context.Canceled
		}

		uri := mi.URI
		if uri == "" {
			continue
		}

		imagePath, ok := facebookalbums.FindImageFile(albumDir, uri, exportRoot, filenameCache)
		if !ok {
			if jpgURI := facebookalbums.TryHEICFallback(uri); jpgURI != "" {
				imagePath, ok = facebookalbums.FindImageFile(albumDir, jpgURI, exportRoot, filenameCache)
				if ok {
					uri = jpgURI
				}
			}
		}
		if !ok {
			if mp3URI := facebookalbums.TryOpusFallback(uri); mp3URI != "" {
				imagePath, ok = facebookalbums.FindImageFile(albumDir, mp3URI, exportRoot, filenameCache)
				if ok {
					uri = mp3URI
				}
			}
		}

		var filename, imageType string
		var imageData []byte

		if ok {
			filename, imageType, imageData = facebookalbums.ReadImageFile(imagePath, uri)
			stats.mu.Lock()
			if len(imageData) > 0 {
				stats.ImagesFound++
			} else {
				stats.ImagesMissing++
			}
			stats.mu.Unlock()
		} else {
			stats.mu.Lock()
			stats.ImagesMissing++
			stats.mu.Unlock()
		}

		creationTs := parseTimestampSec(mi.CreationTimestamp)
		if filename == "" {
			filename = filepath.Base(uri)
		}

		batch = append(batch, importstorage.BatchPostImageItem{
			PostID:            postID,
			URI:               uri,
			Filename:          filename,
			CreationTimestamp: creationTs,
			Title:             mi.Title,
			Description:       mi.Description,
			ImageData:         imageData,
			ImageType:         imageType,
			PostTitle:         entry.Title,
		})

		if len(batch) >= postImageBatchSize {
			flushBatch()
		}
	}

	flushBatch()
	return nil
}

func parsePostsJSON(data []byte) (PostsExport, error) {
	var posts PostsExport
	if err := json.Unmarshal(data, &posts); err != nil {
		return nil, fmt.Errorf("failed to parse posts JSON: %w", err)
	}
	return posts, nil
}

func parseTimestampSec(ts *int64) *time.Time {
	if ts == nil {
		return nil
	}
	t := time.Unix(*ts, 0)
	return &t
}

func extractPostText(entry PostEntry) string {
	for _, d := range entry.Data {
		if d.Post != "" {
			return d.Post
		}
	}
	return ""
}

func extractExternalURL(entry PostEntry) string {
	for _, att := range entry.Attachments {
		for _, d := range att.Data {
			if d.ExternalContext != nil && d.ExternalContext.URL != "" {
				return d.ExternalContext.URL
			}
		}
	}
	return ""
}

func extractMediaItems(entry PostEntry) []PostMediaItem {
	var items []PostMediaItem
	for _, att := range entry.Attachments {
		for _, d := range att.Data {
			if d.Media != nil && d.Media.URI != "" {
				items = append(items, *d.Media)
			}
		}
	}
	return items
}

func determinePostType(entry PostEntry) string {
	hasText := extractPostText(entry) != ""
	hasLink := extractExternalURL(entry) != ""
	hasMedia := len(extractMediaItems(entry)) > 0

	switch {
	case hasMedia && (hasText || hasLink):
		return "mixed"
	case hasMedia:
		return "photo"
	case hasLink:
		return "link"
	default:
		return "status"
	}
}

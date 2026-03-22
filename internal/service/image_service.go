package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
)

// ImageService coordinates media read operations.
type ImageService struct {
	repo *repository.ImageRepo
}

// NewImageService creates an ImageService.
func NewImageService(repo *repository.ImageRepo) *ImageService {
	return &ImageService{repo: repo}
}

// Search returns metadata for images matching the given filters.
func (s *ImageService) Search(ctx context.Context, p model.ImageSearchParams) ([]model.MediaMetadataResponse, error) {
	items, err := s.repo.Search(ctx, p)
	if err != nil {
		return nil, err
	}
	result := make([]model.MediaMetadataResponse, len(items))
	for i, item := range items {
		result[i] = toMediaMetadataResponse(item)
	}
	return result, nil
}

// GetMetadata returns the metadata response for a single media_item. Returns nil, nil if not found.
func (s *ImageService) GetMetadata(ctx context.Context, id int64) (*model.MediaMetadataResponse, error) {
	item, err := s.repo.GetMediaItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	resp := toMediaMetadataResponse(item)
	return &resp, nil
}

// GetFacebookPlaces returns all Facebook-sourced locations.
func (s *ImageService) GetFacebookPlaces(ctx context.Context) ([]model.FacebookPlaceItem, error) {
	return s.repo.GetFacebookPlaces(ctx)
}

// GetDistinctYears returns distinct non-null years from media_items.
func (s *ImageService) GetDistinctYears(ctx context.Context) ([]int, error) {
	return s.repo.GetDistinctYears(ctx)
}

// GetDistinctTags splits all tags columns and returns a sorted, deduped list.
func (s *ImageService) GetDistinctTags(ctx context.Context) ([]string, error) {
	rawTags, err := s.repo.GetAllTagStrings(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	for _, raw := range rawTags {
		for _, tag := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(tag); t != "" {
				set[t] = struct{}{}
			}
		}
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, nil
}

// BulkUpdateTags appends tags to multiple images. Returns updated count and any errors.
func (s *ImageService) BulkUpdateTags(ctx context.Context, imageIDs []int64, tags string) (int, []string) {
	if len(imageIDs) == 0 || strings.TrimSpace(tags) == "" {
		return 0, []string{"image_ids must be non-empty and tags must be non-empty"}
	}
	var updated int
	var errs []string
	for _, id := range imageIDs {
		ok, err := s.repo.UpdateTags(ctx, id, tags)
		if err != nil {
			errs = append(errs, fmt.Sprintf("Error updating image %d: %v", id, err))
			continue
		}
		if ok {
			updated++
		} else {
			errs = append(errs, fmt.Sprintf("Image %d not found", id))
		}
	}
	return updated, errs
}

// UpdateMetadata updates description, tags, and/or rating for one image.
func (s *ImageService) UpdateMetadata(ctx context.Context, id int64, description, tags *string, rating *int) (bool, error) {
	if rating != nil && (*rating < 1 || *rating > 5) {
		return false, fmt.Errorf("rating must be between 1 and 5")
	}
	return s.repo.UpdateMetadata(ctx, id, description, tags, rating)
}

// BulkDeleteImages deletes multiple images by metadata ID.
func (s *ImageService) BulkDeleteImages(ctx context.Context, imageIDs []int64) (int, []string) {
	if len(imageIDs) == 0 {
		return 0, []string{"image_ids must be non-empty"}
	}
	var deleted int
	var errs []string
	for _, id := range imageIDs {
		ok, err := s.repo.DeleteByMetadataID(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Sprintf("Error deleting image %d: %v", id, err))
			continue
		}
		if ok {
			deleted++
		} else {
			errs = append(errs, fmt.Sprintf("Image %d not found", id))
		}
	}
	return deleted, errs
}

// DeleteByMetadataID deletes one image by metadata ID.
func (s *ImageService) DeleteByMetadataID(ctx context.Context, id int64) (bool, error) {
	return s.repo.DeleteByMetadataID(ctx, id)
}

// DeleteByIDRange deletes images by criteria (all, or start_id/end_id range).
func (s *ImageService) DeleteByIDRange(ctx context.Context, all bool, startID, endID *int64) (int64, error) {
	return s.repo.DeleteByIDRange(ctx, all, startID, endID)
}

// GetLocations returns items that have GPS data, shaped for the map view.
func (s *ImageService) GetLocations(ctx context.Context) ([]model.LocationItem, error) {
	items, err := s.repo.GetLocations(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]model.LocationItem, len(items))
	for i, item := range items {
		result[i] = model.LocationItem{
			ID:              item.ID,
			Latitude:        item.Latitude,
			Longitude:       item.Longitude,
			Altitude:        item.Altitude,
			Title:           item.Title,
			Description:     item.Description,
			Year:            item.Year,
			Month:           item.Month,
			Tags:            item.Tags,
			GoogleMapsURL:   item.GoogleMapsURL,
			Region:          item.Region,
			CreatedAt:       item.CreatedAt,
			MediaType:       item.MediaType,
			Source:          item.Source,
			SourceReference: item.SourceReference,
		}
	}
	return result, nil
}

// GetImageContent fetches image bytes.
//
//   - idType "blob"     → look up media_blob by blob ID
//   - idType "metadata" → look up media_blob via media_items.media_blob_id
//
// If preview is true, thumbnail_data is returned (always image/jpeg).
// If image_data is nil and the item has is_referenced=true, the file is read
// from source_reference path on disk (filesystem-referenced images).
//
// HEIC conversion is not implemented in the Go port; HEIC images are returned
// as-is with their original content type. Clients that need JPEG can request
// convert_heic_to_jpg=false explicitly (the parameter is accepted but ignored).
func (s *ImageService) GetImageContent(ctx context.Context, id int64, idType string, preview bool) (*model.ImageContent, error) {
	var blob *model.MediaBlob
	var item *model.MediaItem
	var err error

	if idType == "metadata" {
		blob, err = s.repo.GetBlobByMetadataID(ctx, id)
		if err != nil {
			return nil, err
		}
		if blob == nil {
			return nil, nil // 404
		}
		item, err = s.repo.GetMediaItemByID(ctx, id)
		if err != nil {
			return nil, err
		}
	} else {
		// default: "blob"
		blob, err = s.repo.GetBlobByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if blob == nil {
			return nil, nil // 404
		}
		item, err = s.repo.GetMediaItemByBlobID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	if preview {
		if len(blob.ThumbnailData) == 0 {
			return nil, fmt.Errorf("no thumbnail") // caller maps to 404
		}
		return &model.ImageContent{
			Data:        blob.ThumbnailData,
			ContentType: "image/jpeg",
			Filename:    "image_thumb.jpg",
		}, nil
	}

	data := blob.ImageData

	// Filesystem fallback for referenced images with no stored blob data
	if len(data) == 0 && item != nil && item.IsReferenced && item.SourceReference != nil {
		fileData, err := os.ReadFile(*item.SourceReference)
		if err != nil {
			return nil, fmt.Errorf("referenced image not found at %s: %w", *item.SourceReference, err)
		}
		data = fileData
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no image data") // caller maps to 404
	}

	contentType := "image/jpeg"
	filename := "image"
	if item != nil {
		if item.MediaType != nil && *item.MediaType != "" {
			contentType = *item.MediaType
		}
		if item.SourceReference != nil && *item.SourceReference != "" {
			filename = filepath.Base(*item.SourceReference)
		} else if item.Title != nil && *item.Title != "" {
			filename = *item.Title
		}
	}

	// Convert HEIC to JPEG for browser compatibility
	if isHeic(contentType, filename) {
		jpgData, err := convertHeicToJpeg(data)
		if err != nil {
			return nil, fmt.Errorf("heic conversion: %w", err)
		}
		data = jpgData
		contentType = "image/jpeg"
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".jpg"
	}

	return &model.ImageContent{
		Data:        data,
		ContentType: contentType,
		Filename:    filename,
	}, nil
}

// GetFacebookAlbums returns all albums with their image count.
func (s *ImageService) GetFacebookAlbums(ctx context.Context) ([]model.FacebookAlbumResponse, error) {
	albums, err := s.repo.GetFacebookAlbums(ctx)
	if err != nil {
		return nil, err
	}
	if albums == nil {
		albums = []model.FacebookAlbumResponse{}
	}
	return albums, nil
}

// GetFacebookPosts returns paginated Facebook posts with optional search and post_ids filter.
func (s *ImageService) GetFacebookPosts(ctx context.Context, p repository.GetFacebookPostsParams) (*model.FacebookPostsResponse, error) {
	return s.repo.GetFacebookPosts(ctx, p)
}

// GetPostMediaContent returns image bytes for a Facebook post media item.
// Returns nil, nil if not found or not linked to a post.
func (s *ImageService) GetPostMediaContent(ctx context.Context, mediaID int64) (*model.ImageContent, error) {
	item, err := s.repo.GetPostMediaByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}

	blob, err := s.repo.GetBlobByID(ctx, item.MediaBlobID)
	if err != nil {
		return nil, err
	}
	if blob == nil || len(blob.ImageData) == 0 {
		return nil, fmt.Errorf("no image data")
	}

	contentType := "image/jpeg"
	if item.MediaType != nil && *item.MediaType != "" {
		contentType = *item.MediaType
	} else if item.Title != nil {
		if ct := guessMimeFromFilename(*item.Title); ct != "" {
			contentType = ct
		}
	}

	return &model.ImageContent{
		Data:        blob.ImageData,
		ContentType: contentType,
	}, nil
}

// GetPostMedia returns media items for a Facebook post.
func (s *ImageService) GetPostMedia(ctx context.Context, postID int64) ([]model.FacebookPostMediaItem, error) {
	items, err := s.repo.GetPostMedia(ctx, postID)
	if err != nil {
		return nil, err
	}
	result := make([]model.FacebookPostMediaItem, len(items))
	for i, item := range items {
		var createdAt *string
		if item.CreatedAt != nil {
			s := item.CreatedAt.Format("2006-01-02T15:04:05.999999")
			createdAt = &s
		}
		result[i] = model.FacebookPostMediaItem{
			ID:          item.ID,
			Title:       item.Title,
			Description: item.Description,
			MediaType:   item.MediaType,
			CreatedAt:   createdAt,
		}
	}
	return result, nil
}

// GetAlbumImages returns the image list for a Facebook album.
func (s *ImageService) GetAlbumImages(ctx context.Context, albumID int64) ([]model.AlbumImageItem, error) {
	items, err := s.repo.GetAlbumImages(ctx, albumID)
	if err != nil {
		return nil, err
	}
	result := make([]model.AlbumImageItem, len(items))
	for i, item := range items {
		var createdAt *string
		if item.CreatedAt != nil {
			s := item.CreatedAt.Format("2006-01-02T15:04:05.999999")
			createdAt = &s
		}
		result[i] = model.AlbumImageItem{
			ID:          item.ID,
			Title:       item.Title,
			Description: item.Description,
			MediaType:   item.MediaType,
			CreatedAt:   createdAt,
		}
	}
	return result, nil
}

// GetAlbumImageContent returns image bytes for a Facebook album image.
// Returns nil, nil if the image is not found or not linked to an album.
func (s *ImageService) GetAlbumImageContent(ctx context.Context, imageID int64) (*model.ImageContent, error) {
	item, err := s.repo.GetAlbumImageByID(ctx, imageID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil // 404
	}

	blob, err := s.repo.GetBlobByID(ctx, item.MediaBlobID)
	if err != nil {
		return nil, err
	}
	if blob == nil || len(blob.ImageData) == 0 {
		return nil, fmt.Errorf("no image data") // caller maps to 404
	}

	contentType := "image/jpeg"
	if item.MediaType != nil && *item.MediaType != "" {
		contentType = *item.MediaType
	} else if item.Title != nil {
		// Guess from title extension (mirrors Python mimetypes.guess_type)
		if ct := guessMimeFromFilename(*item.Title); ct != "" {
			contentType = ct
		}
	}

	return &model.ImageContent{
		Data:        blob.ImageData,
		ContentType: contentType,
		Filename:    "",
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toMediaMetadataResponse(m *model.MediaItem) model.MediaMetadataResponse {
	rating := m.Rating
	if rating == 0 {
		rating = 5
	}
	return model.MediaMetadataResponse{
		ID:               m.ID,
		MediaBlobID:      m.MediaBlobID,
		Description:      m.Description,
		Title:            m.Title,
		Author:           m.Author,
		Tags:             m.Tags,
		Categories:       m.Categories,
		Notes:            m.Notes,
		AvailableForTask: m.AvailableForTask,
		MediaType:        m.MediaType,
		Processed:        m.Processed,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
		Year:             m.Year,
		Month:            m.Month,
		Latitude:         m.Latitude,
		Longitude:        m.Longitude,
		Altitude:         m.Altitude,
		Rating:           rating,
		HasGPS:           m.HasGPS,
		GoogleMapsURL:    m.GoogleMapsURL,
		Region:           m.Region,
		Source:           m.Source,
		SourceReference:  m.SourceReference,
	}
}

// guessMimeFromFilename returns a MIME type based on the file extension.
// Mirrors Python's mimetypes.guess_type behaviour for common image formats.
func guessMimeFromFilename(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".heic", ".heif":
		return "image/heic"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	default:
		return ""
	}
}

// isHeic returns true if the content type or filename indicates HEIC/HEIF.
func isHeic(contentType, filename string) bool {
	ct := strings.ToLower(contentType)
	if ct == "image/heic" || ct == "image/heif" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".heic" || ext == ".heif"
}

// convertHeicToJpeg uses ImageMagick to convert HEIC data to JPEG.
// Returns the JPEG bytes or an error if conversion fails.
func convertHeicToJpeg(data []byte) ([]byte, error) {
	tmpIn, err := os.CreateTemp("", "heic-*.heic")
	if err != nil {
		return nil, fmt.Errorf("create temp heic file: %w", err)
	}
	tmpInPath := tmpIn.Name()
	defer os.Remove(tmpInPath)

	if _, err := tmpIn.Write(data); err != nil {
		tmpIn.Close()
		return nil, fmt.Errorf("write temp heic file: %w", err)
	}
	if err := tmpIn.Close(); err != nil {
		return nil, fmt.Errorf("close temp heic file: %w", err)
	}

	tmpOut, err := os.CreateTemp("", "heic-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("create temp jpg file: %w", err)
	}
	tmpOutPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(tmpOutPath)

	// magick input.heic -quality 95 output.jpg
	cmd := exec.Command("magick", tmpInPath, "-quality", "95", tmpOutPath)
	hideConsole(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("imagemagick convert: %w: %s", err, string(out))
	}

	jpgData, err := os.ReadFile(tmpOutPath)
	if err != nil {
		return nil, fmt.Errorf("read converted jpg: %w", err)
	}
	return jpgData, nil
}

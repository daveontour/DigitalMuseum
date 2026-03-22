package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
)

// ErrArtefactUnsupportedMedia is returned when an uploaded file is not an allowed image, PDF, or text/markdown document.
var ErrArtefactUnsupportedMedia = errors.New("unsupported artefact file type")

// ArtefactService orchestrates artefact CRUD and media operations.
type ArtefactService struct {
	repo *repository.ArtefactRepo
}

// NewArtefactService creates an ArtefactService.
func NewArtefactService(repo *repository.ArtefactRepo) *ArtefactService {
	return &ArtefactService{repo: repo}
}

// ── Artefact CRUD ──────────────────────────────────────────────────────────────

func (s *ArtefactService) List(ctx context.Context, search, tags string) ([]*model.ArtefactSummary, error) {
	return s.repo.ListSummaries(ctx, search, tags)
}

func (s *ArtefactService) GetByID(ctx context.Context, id int64) (*model.ArtefactResponse, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}
	items, err := s.repo.GetMediaItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildArtefactResponse(a, items), nil
}

func (s *ArtefactService) Create(ctx context.Context, name string, description, tags, story *string) (*model.ArtefactResponse, error) {
	a, err := s.repo.Create(ctx, name, description, tags, story)
	if err != nil {
		return nil, err
	}
	return buildArtefactResponse(a, nil), nil
}

func (s *ArtefactService) Update(ctx context.Context, id int64, name *string, description, tags, story *string) (*model.ArtefactResponse, error) {
	a, err := s.repo.Update(ctx, id, name, description, tags, story)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}
	items, err := s.repo.GetMediaItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildArtefactResponse(a, items), nil
}

func (s *ArtefactService) Delete(ctx context.Context, id int64) error {
	// Collect artefact-owned media that has no other links
	orphanIDs, err := s.repo.GetOrphanArtefactMediaIDs(ctx, id)
	if err != nil {
		return err
	}

	// Delete blob rows for orphans before deleting the artefact (cascade removes junction rows)
	for _, mid := range orphanIDs {
		blobID, err := s.repo.GetMediaItemBlobID(ctx, mid)
		if err != nil {
			return err
		}
		if err := s.repo.DeleteMediaItem(ctx, mid); err != nil {
			return err
		}
		if blobID != nil {
			if err := s.repo.DeleteMediaBlob(ctx, *blobID); err != nil {
				return err
			}
		}
	}

	return s.repo.Delete(ctx, id)
}

// ── Thumbnail ──────────────────────────────────────────────────────────────────

func (s *ArtefactService) GetThumbnail(ctx context.Context, artefactID int64) ([]byte, error) {
	data, err := s.repo.GetPrimaryBlob(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ── Media upload ───────────────────────────────────────────────────────────────

func (s *ArtefactService) UploadMedia(ctx context.Context, artefactID int64, imageBytes []byte, title, mediaType string) (*model.ArtefactResponse, error) {
	a, err := s.repo.GetByID(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}

	normType := artefactNormalizedMediaType(title, mediaType)
	if err := validateArtefactUpload(title, mediaType); err != nil {
		return nil, err
	}

	var thumbBytes []byte
	if artefactMediaIsImageKind(normType) {
		thumbBytes, _ = makeThumbnail(imageBytes)
	}

	blobID, err := s.repo.InsertMediaBlob(ctx, imageBytes, thumbBytes)
	if err != nil {
		return nil, fmt.Errorf("insert blob: %w", err)
	}

	count, err := s.repo.MediaLinkCount(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	srcRef := fmt.Sprintf("artefact:%d:%d", artefactID, blobID)
	mediaItemID, err := s.repo.InsertMediaItem(ctx, blobID, title, normType, "artefact", srcRef)
	if err != nil {
		return nil, fmt.Errorf("insert media item: %w", err)
	}

	if err := s.repo.LinkMedia(ctx, artefactID, mediaItemID, count); err != nil {
		return nil, fmt.Errorf("link media: %w", err)
	}
	if err := s.repo.TouchUpdatedAt(ctx, artefactID); err != nil {
		return nil, err
	}

	items, err := s.repo.GetMediaItems(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	a2, err := s.repo.GetByID(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	return buildArtefactResponse(a2, items), nil
}

// ── Link/unlink existing media ─────────────────────────────────────────────────

func (s *ArtefactService) LinkMedia(ctx context.Context, artefactID, mediaItemID int64) (*model.ArtefactResponse, error) {
	a, err := s.repo.GetByID(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}

	exists, err := s.repo.MediaLinkExists(ctx, artefactID, mediaItemID)
	if err != nil {
		return nil, err
	}
	if !exists {
		count, err := s.repo.MediaLinkCount(ctx, artefactID)
		if err != nil {
			return nil, err
		}
		if err := s.repo.LinkMedia(ctx, artefactID, mediaItemID, count); err != nil {
			return nil, err
		}
		if err := s.repo.TouchUpdatedAt(ctx, artefactID); err != nil {
			return nil, err
		}
	}

	items, err := s.repo.GetMediaItems(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	a2, err := s.repo.GetByID(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	return buildArtefactResponse(a2, items), nil
}

func (s *ArtefactService) UnlinkMedia(ctx context.Context, artefactID, mediaItemID int64) (*model.ArtefactResponse, error) {
	a, err := s.repo.GetByID(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}

	// Check if this is artefact-owned media with no other links — delete it
	source, err := s.repo.GetMediaItemSource(ctx, mediaItemID)
	if err != nil {
		return nil, err
	}
	deleteMedia := false
	var blobID *int64
	if source == "artefact" {
		otherCount, err := s.repo.OtherArtefactLinkCount(ctx, mediaItemID, artefactID)
		if err != nil {
			return nil, err
		}
		deleteMedia = (otherCount == 0)
		if deleteMedia {
			blobID, err = s.repo.GetMediaItemBlobID(ctx, mediaItemID)
			if err != nil {
				return nil, err
			}
		}
	}

	if err := s.repo.UnlinkMedia(ctx, artefactID, mediaItemID); err != nil {
		return nil, err
	}

	if deleteMedia {
		if err := s.repo.DeleteMediaItem(ctx, mediaItemID); err != nil {
			return nil, err
		}
		if blobID != nil {
			if err := s.repo.DeleteMediaBlob(ctx, *blobID); err != nil {
				return nil, err
			}
		}
	}

	if err := s.repo.TouchUpdatedAt(ctx, artefactID); err != nil {
		return nil, err
	}

	items, err := s.repo.GetMediaItems(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	a2, err := s.repo.GetByID(ctx, artefactID)
	if err != nil {
		return nil, err
	}
	return buildArtefactResponse(a2, items), nil
}

// ── Export / Import ────────────────────────────────────────────────────────────

func (s *ArtefactService) ExportAll(ctx context.Context) ([]*repository.ArtefactExportRow, error) {
	return s.repo.ExportAll(ctx)
}

func (s *ArtefactService) ImportArtefacts(ctx context.Context, artefacts []map[string]any) (created, linked, skipped int, err error) {
	for _, item := range artefacts {
		name, _ := item["name"].(string)
		if name == "" {
			continue
		}
		desc := optStr(item["description"])
		tags := optStr(item["tags"])
		story := optStr(item["story"])

		a, err2 := s.repo.Create(ctx, name, desc, tags, story)
		if err2 != nil {
			return created, linked, skipped, err2
		}
		created++

		refs, _ := item["media_refs"].([]any)
		for _, r := range refs {
			ref, ok := r.(map[string]any)
			if !ok {
				skipped++
				continue
			}
			src, _ := ref["source"].(string)
			srcRef, _ := ref["source_reference"].(string)
			if src == "" || srcRef == "" {
				skipped++
				continue
			}
			mediaItemID, err2 := s.repo.FindMediaBySrcRef(ctx, src, srcRef)
			if err2 != nil {
				return created, linked, skipped, err2
			}
			if mediaItemID == 0 {
				skipped++
				continue
			}
			exists, err2 := s.repo.MediaLinkExists(ctx, a.ID, mediaItemID)
			if err2 != nil {
				return created, linked, skipped, err2
			}
			if !exists {
				sortOrder := 0
				if v, ok := ref["sort_order"].(float64); ok {
					sortOrder = int(v)
				}
				if err2 := s.repo.LinkMedia(ctx, a.ID, mediaItemID, sortOrder); err2 != nil {
					return created, linked, skipped, err2
				}
				linked++
			}
		}
	}
	return created, linked, skipped, nil
}

// ── helpers ────────────────────────────────────────────────────────────────────

func buildArtefactResponse(a *model.Artefact, items []*model.ArtefactMediaItem) *model.ArtefactResponse {
	if items == nil {
		items = []*model.ArtefactMediaItem{}
	}
	mediaItems := make([]model.ArtefactMediaItem, len(items))
	for i, it := range items {
		mediaItems[i] = *it
	}
	return &model.ArtefactResponse{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		Tags:        a.Tags,
		Story:       a.Story,
		CreatedAt:   a.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		UpdatedAt:   a.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		MediaItems:  mediaItems,
	}
}

// makeThumbnail generates a JPEG thumbnail (max 400×400) from raw image bytes.
func makeThumbnail(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	maxSize := 400
	if w > maxSize || h > maxSize {
		ratio := float64(w) / float64(h)
		if w > h {
			w = maxSize
			h = int(float64(maxSize) / ratio)
		} else {
			h = maxSize
			w = int(float64(maxSize) * ratio)
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func optStr(v any) *string {
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}

func artefactNormalizedMediaType(filename, header string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	h := strings.ToLower(strings.TrimSpace(header))
	if semi := strings.Index(h, ";"); semi >= 0 {
		h = strings.TrimSpace(h[:semi])
	}
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".heic", ".heif":
		return "image/heic"
	case ".bmp":
		return "image/bmp"
	}
	if strings.HasPrefix(h, "image/") && h != "" {
		return header
	}
	if h == "application/pdf" || h == "text/plain" || h == "text/markdown" {
		return header
	}
	return header
}

func artefactMediaIsImageKind(normType string) bool {
	base := strings.ToLower(strings.TrimSpace(normType))
	if i := strings.Index(base, ";"); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	return strings.HasPrefix(base, "image/")
}

func validateArtefactUpload(filename, header string) error {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	h := strings.ToLower(strings.TrimSpace(header))
	if semi := strings.Index(h, ";"); semi >= 0 {
		h = strings.TrimSpace(h[:semi])
	}
	switch ext {
	case ".pdf", ".md", ".markdown", ".txt":
		return nil
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic", ".heif":
		return nil
	}
	if strings.HasPrefix(h, "image/") {
		return nil
	}
	if h == "application/pdf" || h == "text/plain" || h == "text/markdown" {
		return nil
	}
	return fmt.Errorf("%w — use an image, PDF, Markdown (.md), or plain text (.txt)", ErrArtefactUnsupportedMedia)
}

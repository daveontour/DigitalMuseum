package importstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const facebookPostSource = "facebook_post"

// BatchPostImageItem represents a single post media item for batch save.
type BatchPostImageItem struct {
	PostID            int64
	URI               string
	Filename          string
	CreationTimestamp *time.Time
	Title             string
	Description       string
	ImageData         []byte
	ImageType         string
	PostTitle         string
}

// FacebookPostStorage handles Facebook post storage operations.
type FacebookPostStorage struct {
	pool *sql.DB
}

// NewFacebookPostStorage creates a new Facebook post storage instance.
func NewFacebookPostStorage(pool *sql.DB) *FacebookPostStorage {
	return &FacebookPostStorage{pool: pool}
}

// FindPostByTimestampAndTitle looks up a post by its Unix timestamp and title.
func (s *FacebookPostStorage) FindPostByTimestampAndTitle(ctx context.Context, ts *time.Time, title string) (int64, bool, error) {
	var postID int64
	err := s.pool.QueryRowContext(ctx,
		`SELECT id FROM facebook_posts WHERE timestamp = ?1 AND title = ?2 LIMIT 1`,
		ts, title,
	).Scan(&postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to find post: %w", err)
	}
	return postID, true, nil
}

// SaveOrUpdatePost creates a new post or returns the existing one.
func (s *FacebookPostStorage) SaveOrUpdatePost(
	ctx context.Context,
	ts *time.Time,
	title, postText, externalURL, postType string,
) (int64, bool, error) {
	uid := uidFromCtx(ctx)

	existingID, found, err := s.FindPostByTimestampAndTitle(ctx, ts, title)
	if err != nil {
		return 0, false, err
	}
	if found {
		return existingID, false, nil
	}

	var postID int64
	err = s.pool.QueryRowContext(ctx,
		`INSERT INTO facebook_posts (timestamp, title, post_text, external_url, post_type, user_id, created_at, updated_at)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id`,
		ts,
		nullIfEmpty(title),
		nullIfEmpty(postText),
		nullIfEmpty(externalURL),
		nullIfEmpty(postType),
		uidVal(uid),
	).Scan(&postID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to insert post: %w", err)
	}
	return postID, true, nil
}

// SavePostImagesBatch saves multiple post media items in a single transaction.
func (s *FacebookPostStorage) SavePostImagesBatch(ctx context.Context, items []BatchPostImageItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	uid := uidFromCtx(ctx)

	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	imported := 0
	for _, item := range items {
		// Unique key per photo so re-imports do not duplicate (post_id:uri) —
		// mirrors albumPhotoSourceRef; PostID alone collides across every
		// photo in the same multi-photo post.
		sourceRef := postPhotoSourceRef(item.PostID, item.URI)

		var existingMediaItemID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM media_items WHERE source = ?1 AND source_reference = ?2 LIMIT 1`,
			facebookPostSource, sourceRef).Scan(&existingMediaItemID)
		if err == nil {
			// Photo already exists from a previous import; ensure post_media link exists.
			var linkCount int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM post_media WHERE post_id = ?1 AND media_item_id = ?2`,
				item.PostID, existingMediaItemID).Scan(&linkCount); err != nil {
				return imported, fmt.Errorf("failed to check post_media for %s: %w", item.URI, err)
			}
			if linkCount == 0 {
				_, err = tx.ExecContext(ctx, `INSERT INTO post_media (post_id, media_item_id) VALUES (?1, ?2)`, item.PostID, existingMediaItemID)
				if err != nil {
					return imported, fmt.Errorf("failed to link existing media item to post for %s: %w", item.URI, err)
				}
			}
			imported++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return imported, fmt.Errorf("failed to check existing media item for %s: %w", item.URI, err)
		}

		var blobID int64
		if len(item.ImageData) > 0 {
			err = tx.QueryRowContext(ctx,
				`INSERT INTO media_blobs (image_data, thumbnail_data, user_id) VALUES (?1, ?2, ?3) RETURNING id`,
				item.ImageData, nil, uidVal(uid),
			).Scan(&blobID)
		} else {
			err = tx.QueryRowContext(ctx,
				`INSERT INTO media_blobs (image_data, thumbnail_data, user_id) VALUES (?1, ?2, ?3) RETURNING id`,
				nil, nil, uidVal(uid),
			).Scan(&blobID)
		}
		if err != nil {
			return imported, fmt.Errorf("failed to insert media blob for %s: %w", item.URI, err)
		}

		var year, month *int
		if item.CreationTimestamp != nil {
			y := item.CreationTimestamp.Year()
			m := int(item.CreationTimestamp.Month())
			year = &y
			month = &m
		}

		displayTitle := item.Title
		if displayTitle == "" {
			displayTitle = item.Filename
		}

		var mediaItemID int64
		err = tx.QueryRowContext(ctx, `INSERT INTO media_items (
			media_blob_id, tags, source, source_reference, title, description,
			media_type, year, month, latitude, longitude, altitude, has_gps,
			processed, available_for_task, rating, is_personal, is_business,
			is_social, is_promotional, is_spam, is_important, user_id, created_at, updated_at, is_referenced
		) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, FALSE)
		RETURNING id`,
			blobID,
			nullIfEmpty(item.PostTitle),
			facebookPostSource,
			sourceRef,
			nullIfEmpty(displayTitle),
			nullIfEmpty(item.Description),
			nullIfEmpty(item.ImageType),
			year, month,
			nil, nil, nil,
			false, false, false, 5,
			false, false, false, false, false, false,
			uidVal(uid),
		).Scan(&mediaItemID)
		if err != nil {
			return imported, fmt.Errorf("failed to insert media item for %s: %w", item.URI, err)
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO post_media (post_id, media_item_id) VALUES (?1, ?2)`,
			item.PostID, mediaItemID,
		)
		if err != nil {
			return imported, fmt.Errorf("failed to insert post_media for %s: %w", item.URI, err)
		}
		imported++
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}
	return imported, nil
}

// postPhotoSourceRef returns a unique key for a Facebook post photo so
// re-imports can skip duplicates — mirrors albumPhotoSourceRef in
// facebook_album_storage.go. PostID is stable across reimports since
// SaveOrUpdatePost reuses an existing post's id by timestamp+title match.
func postPhotoSourceRef(postID int64, uri string) string {
	s := strconv.FormatInt(postID, 10) + ":" + strings.TrimSpace(uri)
	if len(s) > maxSourceRefLen {
		return s[:maxSourceRefLen]
	}
	return s
}

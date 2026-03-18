package importstorage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool *pgxpool.Pool
}

// NewFacebookPostStorage creates a new Facebook post storage instance.
func NewFacebookPostStorage(pool *pgxpool.Pool) *FacebookPostStorage {
	return &FacebookPostStorage{pool: pool}
}

// FindPostByTimestampAndTitle looks up a post by its Unix timestamp and title.
func (s *FacebookPostStorage) FindPostByTimestampAndTitle(ctx context.Context, ts *time.Time, title string) (int64, bool, error) {
	var postID int64
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM facebook_posts WHERE timestamp = $1 AND title = $2 LIMIT 1`,
		ts, title,
	).Scan(&postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	existingID, found, err := s.FindPostByTimestampAndTitle(ctx, ts, title)
	if err != nil {
		return 0, false, err
	}
	if found {
		return existingID, false, nil
	}

	var postID int64
	err = s.pool.QueryRow(ctx,
		`INSERT INTO facebook_posts (timestamp, title, post_text, external_url, post_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING id`,
		ts,
		nullIfEmpty(title),
		nullIfEmpty(postText),
		nullIfEmpty(externalURL),
		nullIfEmpty(postType),
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	imported := 0
	for _, item := range items {
		var blobID int64
		if len(item.ImageData) > 0 {
			err = tx.QueryRow(ctx,
				`INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, $2) RETURNING id`,
				item.ImageData, nil,
			).Scan(&blobID)
		} else {
			err = tx.QueryRow(ctx,
				`INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, $2) RETURNING id`,
				nil, nil,
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
		err = tx.QueryRow(ctx, `INSERT INTO media_items (
			media_blob_id, tags, source, source_reference, title, description,
			media_type, year, month, latitude, longitude, altitude, has_gps,
			processed, available_for_task, rating, is_personal, is_business,
			is_social, is_promotional, is_spam, is_important, created_at, updated_at, is_referenced
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW(), NOW(), FALSE)
		RETURNING id`,
			blobID,
			nullIfEmpty(item.PostTitle),
			facebookPostSource,
			fmt.Sprintf("%d", item.PostID),
			nullIfEmpty(displayTitle),
			nullIfEmpty(item.Description),
			nullIfEmpty(item.ImageType),
			year, month,
			nil, nil, nil,
			false, false, false, 5,
			false, false, false, false, false, false,
		).Scan(&mediaItemID)
		if err != nil {
			return imported, fmt.Errorf("failed to insert media item for %s: %w", item.URI, err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO post_media (post_id, media_item_id) VALUES ($1, $2)`,
			item.PostID, mediaItemID,
		)
		if err != nil {
			return imported, fmt.Errorf("failed to insert post_media for %s: %w", item.URI, err)
		}
		imported++
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}
	return imported, nil
}

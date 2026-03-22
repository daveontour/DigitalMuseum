package importstorage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const filesystemSource = "filesystem"

// BatchImageItem represents a single image for batch save
type BatchImageItem struct {
	SourceRef    string
	ImageData    []byte // nil for referenced images
	MediaType    string
	Title        string
	Tags         string
	IsReferenced bool
}

// ImageStorage handles filesystem image storage operations for imports.
type ImageStorage struct {
	pool *pgxpool.Pool
}

// NewImageStorage creates a new image storage instance
func NewImageStorage(pool *pgxpool.Pool) *ImageStorage {
	return &ImageStorage{pool: pool}
}

// SaveImage saves or updates an image in the database.
// Deduplication is by source + source_reference.
// Returns (media_item_id, is_update, error)
func (s *ImageStorage) SaveImage(ctx context.Context, sourceRef string, imageData []byte, mediaType, title, tags string, isReferenced bool) (int64, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, "SET LOCAL synchronous_commit = off"); err != nil {
		return 0, false, fmt.Errorf("failed to set synchronous_commit: %w", err)
	}

	var existingID int64
	var existingBlobID int64
	checkQuery := `SELECT id, media_blob_id FROM media_items WHERE source = $1 AND source_reference = $2 LIMIT 1`
	err = tx.QueryRow(ctx, checkQuery, filesystemSource, sourceRef).Scan(&existingID, &existingBlobID)

	if err == nil {
		if !isReferenced {
			updateBlobQuery := `UPDATE media_blobs SET image_data = $1, thumbnail_data = NULL, updated_at = NOW() WHERE id = $2`
			_, err = tx.Exec(ctx, updateBlobQuery, imageData, existingBlobID)
			if err != nil {
				return 0, false, fmt.Errorf("failed to update media blob: %w", err)
			}
		}

		updateMetaQuery := `UPDATE media_items SET title = $1, tags = $2, media_type = $3, is_referenced = $4, updated_at = NOW() WHERE id = $5`
		_, err = tx.Exec(ctx, updateMetaQuery, title, nullIfEmpty(tags), nullIfEmpty(mediaType), isReferenced, existingID)
		if err != nil {
			return 0, false, fmt.Errorf("failed to update media item: %w", err)
		}

		if err = tx.Commit(ctx); err != nil {
			return 0, false, fmt.Errorf("failed to commit: %w", err)
		}
		return existingID, true, nil
	}

	if err != pgx.ErrNoRows {
		return 0, false, fmt.Errorf("failed to check for existing image: %w", err)
	}

	var blobID int64
	insertBlobQuery := `INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, $2) RETURNING id`
	err = tx.QueryRow(ctx, insertBlobQuery, imageData, nil).Scan(&blobID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to insert media blob: %w", err)
	}

	var mediaItemID int64
	insertMetaQuery := `INSERT INTO media_items (
		media_blob_id, tags, source, source_reference, title, description,
		media_type, year, month, latitude, longitude, altitude, has_gps,
		processed, available_for_task, rating, is_personal, is_business,
		is_social, is_promotional, is_spam, is_important, is_referenced, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, NOW(), NOW())
	RETURNING id`

	err = tx.QueryRow(ctx, insertMetaQuery,
		blobID,
		nullIfEmpty(tags),
		filesystemSource,
		sourceRef,
		nullIfEmpty(title),
		nil,
		nullIfEmpty(mediaType),
		nil, nil, nil, nil, nil,
		false, false, false, 5,
		false, false, false, false, false, false, false,
		isReferenced,
	).Scan(&mediaItemID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to insert media item: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("failed to commit: %w", err)
	}
	return mediaItemID, false, nil
}

// SaveImagesBatch saves or updates multiple images in a single transaction.
// Returns (importedCount, updatedCount, error). On error, the entire batch is rolled back.
func (s *ImageStorage) SaveImagesBatch(ctx context.Context, items []BatchImageItem) (imported, updated int, err error) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, "SET LOCAL synchronous_commit = off"); err != nil {
		return 0, 0, fmt.Errorf("failed to set synchronous_commit: %w", err)
	}

	sourceRefs := make([]string, len(items))
	for i := range items {
		sourceRefs[i] = items[i].SourceRef
	}

	existing := make(map[string]struct{ id, blobID int64 })
	rows, err := tx.Query(ctx,
		`SELECT source_reference, id, media_blob_id FROM media_items WHERE source = $1 AND source_reference = ANY($2)`,
		filesystemSource, sourceRefs)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to check existing images: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ref string
		var id, blobID int64
		if err := rows.Scan(&ref, &id, &blobID); err != nil {
			return 0, 0, fmt.Errorf("failed to scan existing row: %w", err)
		}
		existing[ref] = struct{ id, blobID int64 }{id: id, blobID: blobID}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("failed iterating existing rows: %w", err)
	}

	for _, item := range items {
		if ex, ok := existing[item.SourceRef]; ok {
			if !item.IsReferenced {
				_, err = tx.Exec(ctx, `UPDATE media_blobs SET image_data = $1, thumbnail_data = NULL, updated_at = NOW() WHERE id = $2`,
					item.ImageData, ex.blobID)
				if err != nil {
					return 0, 0, fmt.Errorf("failed to update media blob for %s: %w", item.SourceRef, err)
				}
			}
			_, err = tx.Exec(ctx, `UPDATE media_items SET title = $1, tags = $2, media_type = $3, is_referenced = $4, updated_at = NOW() WHERE id = $5`,
				nullIfEmpty(item.Title), nullIfEmpty(item.Tags), nullIfEmpty(item.MediaType), item.IsReferenced, ex.id)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to update media item for %s: %w", item.SourceRef, err)
			}
			updated++
		} else {
			var blobID int64
			err = tx.QueryRow(ctx, `INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, $2) RETURNING id`,
				item.ImageData, nil).Scan(&blobID)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to insert media blob for %s: %w", item.SourceRef, err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO media_items (
				media_blob_id, tags, source, source_reference, title, description,
				media_type, year, month, latitude, longitude, altitude, has_gps,
				processed, available_for_task, rating, is_personal, is_business,
				is_social, is_promotional, is_spam, is_important, is_referenced, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, NOW(), NOW())`,
				blobID,
				nullIfEmpty(item.Tags),
				filesystemSource,
				item.SourceRef,
				nullIfEmpty(item.Title),
				nil,
				nullIfEmpty(item.MediaType),
				nil, nil, nil, nil, nil,
				false, false, false, 5,
				false, false, false, false, false, false,
				item.IsReferenced,
			)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to insert media item for %s: %w", item.SourceRef, err)
			}
			imported++
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("failed to commit: %w", err)
	}
	return imported, updated, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

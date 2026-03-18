package thumbnails

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

const progressCallbackInterval = 25

// ImportStats holds statistics about the thumbnail processing.
type ImportStats struct {
	TotalItems   int
	Processed    int64
	Errors       int64
	CurrentItem  string
	mu           sync.Mutex
}

func (s *ImportStats) copyStats() ImportStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ImportStats{
		TotalItems:  s.TotalItems,
		Processed:   s.Processed,
		Errors:      s.Errors,
		CurrentItem: s.CurrentItem,
	}
}

// ProgressCallback is called periodically during processing.
type ProgressCallback func(ImportStats)

// CancelledCheck returns true if processing should stop.
type CancelledCheck func() bool

type mediaItemWork struct {
	MediaItemID     int64
	BlobID          int64
	MediaType       *string
	IsReferenced    bool
	SourceReference *string
}

type processResult struct {
	Success bool
	Error   error
}

// ProcessThumbnailsAndExif processes media items: generates thumbnails and extracts EXIF.
func ProcessThumbnailsAndExif(
	ctx context.Context,
	pool *pgxpool.Pool,
	reprocess bool,
	progressCallback ProgressCallback,
	cancelledCheck CancelledCheck,
) (*ImportStats, error) {
	stats := &ImportStats{}

	var query string
	if reprocess {
		query = `
		SELECT
			mi.id as media_item_id,
			mi.media_blob_id,
			mi.media_type,
			mi.is_referenced,
			mi.source_reference
		FROM media_items mi
		INNER JOIN media_blobs mb ON mi.media_blob_id = mb.id
		WHERE (mb.image_data IS NOT NULL AND LENGTH(mb.image_data) > 0)
		   OR mi.is_referenced = true
		ORDER BY mi.id
		`
	} else {
		query = `
		SELECT
			mi.id as media_item_id,
			mi.media_blob_id,
			mi.media_type,
			mi.is_referenced,
			mi.source_reference
		FROM media_items mi
		INNER JOIN media_blobs mb ON mi.media_blob_id = mb.id
		WHERE mi.processed = false
		   OR mb.thumbnail_data IS NULL
		   OR LENGTH(mb.thumbnail_data) = 0
		ORDER BY mi.id
		`
	}

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query media items: %w", err)
	}
	defer rows.Close()

	var workItems []mediaItemWork
	var mediaItemID, blobID int64
	var mediaType, sourceReference *string
	var isReferenced bool

	for rows.Next() {
		if cancelledCheck != nil && cancelledCheck() {
			return stats, nil
		}
		if err := rows.Scan(&mediaItemID, &blobID, &mediaType, &isReferenced, &sourceReference); err != nil {
			continue
		}

		if mediaType == nil || !strings.HasPrefix(strings.ToLower(*mediaType), "image/") {
			continue
		}

		workItems = append(workItems, mediaItemWork{
			MediaItemID:     mediaItemID,
			BlobID:          blobID,
			MediaType:       mediaType,
			IsReferenced:    isReferenced,
			SourceReference: sourceReference,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	if len(workItems) == 0 {
		return stats, nil
	}

	stats.mu.Lock()
	stats.TotalItems = len(workItems)
	stats.mu.Unlock()

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	workChan := make(chan mediaItemWork, len(workItems))
	resultChan := make(chan processResult, len(workItems))

	var processedCount, errorCount int64
	var statsMutex sync.Mutex
	processor := &Processor{}

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			for work := range workChan {
				if cancelledCheck != nil && cancelledCheck() {
					continue
				}
				result := processMediaItem(ctx, pool, processor, work)
				resultChan <- result

				statsMutex.Lock()
				if result.Success {
					processedCount++
					stats.mu.Lock()
					stats.Processed = processedCount
					stats.Errors = errorCount
					stats.CurrentItem = fmt.Sprintf("media_item %d", work.MediaItemID)
					stats.mu.Unlock()
					if processedCount%progressCallbackInterval == 0 && progressCallback != nil {
						progressCallback(stats.copyStats())
					}
				} else {
					errorCount++
					stats.mu.Lock()
					stats.Errors = errorCount
					stats.mu.Unlock()
				}
				statsMutex.Unlock()
			}
		}(i)
	}

	sent := 0
	for _, work := range workItems {
		if cancelledCheck != nil && cancelledCheck() {
			break
		}
		workChan <- work
		sent++
	}
	close(workChan)

	// Wait for workers to finish
	for i := 0; i < sent; i++ {
		<-resultChan
	}

	stats.mu.Lock()
	stats.Processed = processedCount
	stats.Errors = errorCount
	stats.mu.Unlock()

	return stats, nil
}

func processMediaItem(ctx context.Context, pool *pgxpool.Pool, processor *Processor, work mediaItemWork) processResult {
	var imageData []byte

	if work.IsReferenced {
		if work.SourceReference == nil || *work.SourceReference == "" {
			return processResult{Success: false, Error: fmt.Errorf("referenced media_item_id=%d has no source_reference path", work.MediaItemID)}
		}
		var err error
		imageData, err = os.ReadFile(*work.SourceReference)
		if err != nil {
			return processResult{Success: false, Error: fmt.Errorf("failed to read referenced file %s for media_item_id=%d: %w",
				*work.SourceReference, work.MediaItemID, err)}
		}
	} else {
		loadImageQuery := `SELECT image_data FROM media_blobs WHERE id = $1`
		err := pool.QueryRow(ctx, loadImageQuery, work.BlobID).Scan(&imageData)
		if err != nil {
			return processResult{Success: false, Error: fmt.Errorf("failed to load image data for blob_id=%d: %w",
				work.BlobID, err)}
		}
	}

	if len(imageData) == 0 {
		return processResult{Success: false, Error: fmt.Errorf("image data is empty for media_item_id=%d", work.MediaItemID)}
	}

	thumbData, exifData, err := processor.CreateThumbAndGetExif(imageData, true, true, 200)
	if err != nil {
		return processResult{Success: false, Error: fmt.Errorf("CreateThumbAndGetExif failed for media_item_id=%d blob_id=%d: %w",
			work.MediaItemID, work.BlobID, err)}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return processResult{Success: false, Error: fmt.Errorf("failed to begin transaction: %w", err)}
	}
	defer tx.Rollback(ctx)

	if thumbData != nil {
		updateBlobQuery := `UPDATE media_blobs SET thumbnail_data = $1 WHERE id = $2`
		_, err = tx.Exec(ctx, updateBlobQuery, thumbData, work.BlobID)
		if err != nil {
			return processResult{Success: false, Error: fmt.Errorf("failed to update thumbnail: %w", err)}
		}
	}

	updateItemQuery := `UPDATE media_items 
		SET processed = true, 
			description = COALESCE(NULLIF($1, ''), description),
			year = COALESCE($2, year),
			month = COALESCE($3, month),
			latitude = COALESCE($4, latitude),
			longitude = COALESCE($5, longitude),
			has_gps = COALESCE($6, has_gps),
			updated_at = NOW()
		WHERE id = $7`

	var description *string
	var year, month *int
	var latitude, longitude *float64
	var hasGPS *bool

	if exifData != nil {
		if exifData.Description != "" {
			description = &exifData.Description
		}
		if exifData.DateTaken != "" {
			parts := strings.Fields(exifData.DateTaken)
			if len(parts) > 0 {
				dateParts := strings.Split(parts[0], ":")
				if len(dateParts) >= 2 {
					var y, m int
					if _, err := fmt.Sscanf(dateParts[0], "%d", &y); err == nil {
						year = &y
					}
					if _, err := fmt.Sscanf(dateParts[1], "%d", &m); err == nil {
						month = &m
					}
				}
			}
		}
		if exifData.LatitudeDecimal != nil && exifData.LongitudeDecimal != nil {
			latitude = exifData.LatitudeDecimal
			longitude = exifData.LongitudeDecimal
			hasGPSVal := true
			hasGPS = &hasGPSVal
		}
	}

	_, err = tx.Exec(ctx, updateItemQuery, description, year, month, latitude, longitude, hasGPS, work.MediaItemID)
	if err != nil {
		return processResult{Success: false, Error: fmt.Errorf("failed to update media item: %w", err)}
	}

	if err = tx.Commit(ctx); err != nil {
		return processResult{Success: false, Error: fmt.Errorf("failed to commit transaction: %w", err)}
	}

	return processResult{Success: true}
}

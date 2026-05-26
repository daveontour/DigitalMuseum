package importstorage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/georegion"
)

const coordTolerance = 0.0001 // ~11 meters

// FacebookPlacesStorage handles location storage for Facebook places.
type FacebookPlacesStorage struct {
	pool *sql.DB
}

// NewFacebookPlacesStorage creates a new Facebook places storage instance.
func NewFacebookPlacesStorage(pool *sql.DB) *FacebookPlacesStorage {
	return &FacebookPlacesStorage{pool: pool}
}

// SaveOrUpdateLocation saves or updates a location. Deduplication by name + coordinates + user_id.
func (s *FacebookPlacesStorage) SaveOrUpdateLocation(ctx context.Context, name, address string, latitude, longitude *float64, source, sourceRef string) (created bool, err error) {
	uid := uidFromCtx(ctx)

	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var existingID int64

	if latitude != nil && longitude != nil {
		err = tx.QueryRowContext(ctx, `SELECT id FROM locations WHERE name = ?1
			AND latitude IS NOT NULL AND longitude IS NOT NULL
			AND ABS(latitude - ?2) < ?4 AND ABS(longitude - ?3) < ?4
			AND (user_id = ?5 OR (?5 IS NULL AND user_id IS NULL))
			LIMIT 1`,
			name, *latitude, *longitude, coordTolerance, uidVal(uid)).Scan(&existingID)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT id FROM locations WHERE name = ?1
			AND latitude IS NULL AND longitude IS NULL
			AND (user_id = ?2 OR (?2 IS NULL AND user_id IS NULL))
			LIMIT 1`,
			name, uidVal(uid)).Scan(&existingID)
	}

	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE locations SET
			address = COALESCE(?1, address),
			latitude = COALESCE(?2, latitude),
			longitude = COALESCE(?3, longitude),
			source = ?4,
			source_reference = COALESCE(?5, source_reference),
			updated_at = CURRENT_TIMESTAMP
			WHERE id = ?6`,
			nullIfEmpty(address), latitude, longitude, source, nullIfEmpty(sourceRef), existingID)
		if err != nil {
			return false, fmt.Errorf("failed to update location: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit: %w", err)
		}
		return false, nil
	}

	if err != sql.ErrNoRows {
		return false, fmt.Errorf("failed to check existing location: %w", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO locations (name, address, latitude, longitude, source, source_reference, user_id, created_at, updated_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		name, nullIfEmpty(address), latitude, longitude, source, nullIfEmpty(sourceRef), uidVal(uid))
	if err != nil {
		return false, fmt.Errorf("failed to insert location: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit: %w", err)
	}
	return true, nil
}

// UpdateLocationRegions sets locations.region from latitude/longitude bounding boxes.
func (s *FacebookPlacesStorage) UpdateLocationRegions(ctx context.Context) error {
	if err := georegion.UpdateLocationRegions(ctx, s.pool); err != nil {
		return fmt.Errorf("update_location_regions failed: %w", err)
	}
	return nil
}

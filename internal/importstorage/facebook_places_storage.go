package importstorage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const coordTolerance = 0.0001 // ~11 meters

// FacebookPlacesStorage handles location storage for Facebook places.
type FacebookPlacesStorage struct {
	pool *pgxpool.Pool
}

// NewFacebookPlacesStorage creates a new Facebook places storage instance.
func NewFacebookPlacesStorage(pool *pgxpool.Pool) *FacebookPlacesStorage {
	return &FacebookPlacesStorage{pool: pool}
}

// SaveOrUpdateLocation saves or updates a location. Deduplication by name + coordinates.
func (s *FacebookPlacesStorage) SaveOrUpdateLocation(ctx context.Context, name, address string, latitude, longitude *float64, source, sourceRef string) (created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingID int64

	if latitude != nil && longitude != nil {
		err = tx.QueryRow(ctx, `SELECT id FROM locations WHERE name = $1
			AND latitude IS NOT NULL AND longitude IS NOT NULL
			AND ABS(latitude - $2) < $4 AND ABS(longitude - $3) < $4
			LIMIT 1`,
			name, *latitude, *longitude, coordTolerance).Scan(&existingID)
	} else {
		err = tx.QueryRow(ctx, `SELECT id FROM locations WHERE name = $1
			AND latitude IS NULL AND longitude IS NULL
			LIMIT 1`,
			name).Scan(&existingID)
	}

	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE locations SET
			address = COALESCE($1, address),
			latitude = COALESCE($2, latitude),
			longitude = COALESCE($3, longitude),
			source = $4,
			source_reference = COALESCE($5, source_reference),
			updated_at = NOW()
			WHERE id = $6`,
			nullIfEmpty(address), latitude, longitude, source, nullIfEmpty(sourceRef), existingID)
		if err != nil {
			return false, fmt.Errorf("failed to update location: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("failed to commit: %w", err)
		}
		return false, nil
	}

	if err != pgx.ErrNoRows {
		return false, fmt.Errorf("failed to check existing location: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO locations (name, address, latitude, longitude, source, source_reference, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		name, nullIfEmpty(address), latitude, longitude, source, nullIfEmpty(sourceRef))
	if err != nil {
		return false, fmt.Errorf("failed to insert location: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit: %w", err)
	}
	return true, nil
}

// UpdateLocationRegions calls the database function update_location_regions().
func (s *FacebookPlacesStorage) UpdateLocationRegions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "SELECT update_location_regions()")
	if err != nil {
		return fmt.Errorf("update_location_regions failed: %w", err)
	}
	return nil
}

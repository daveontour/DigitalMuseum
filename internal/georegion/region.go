package georegion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/daveontour/aimuseum/internal/appctx"
)

var ErrRegionRecalcCancelled = errors.New("region recalculation cancelled")

func inRegionBBox(lat, lng float64, bbox [4]float64) bool {
	minLon, minLat, maxLon, maxLat := bbox[0], bbox[1], bbox[2], bbox[3]
	return lng >= minLon && lat >= minLat && lng <= maxLon && lat <= maxLat
}

// UpdateMediaItemRegions sets media_items.region for rows with GPS coordinates.
func UpdateMediaItemRegions(ctx context.Context, db *sql.DB) error {
	_, err := updateTableRegions(ctx, db, "media_items", "", nil)
	return err
}

// UpdateLocationRegions sets locations.region for rows with GPS coordinates.
func UpdateLocationRegions(ctx context.Context, db *sql.DB) error {
	_, err := updateTableRegions(ctx, db, "locations", "", nil)
	return err
}

// RegionRecalcOptions configures progress reporting and cancellation for region recalculation.
type RegionRecalcOptions struct {
	Progress func(processed, total int)
	Cancel   func() bool
}

// RecalculateMediaItemRegions updates media_items.region from latitude/longitude for the
// authenticated user (when user_id is present in ctx). Returns the number of rows updated.
func RecalculateMediaItemRegions(ctx context.Context, db *sql.DB, opts *RegionRecalcOptions) (int, error) {
	return updateTableRegions(ctx, db, "media_items", "", opts)
}

// RecalculateImageRegions updates region for image media_items that have GPS coordinates.
func RecalculateImageRegions(ctx context.Context, db *sql.DB, opts *RegionRecalcOptions) (int, error) {
	return updateTableRegions(ctx, db, "media_items", ` AND media_type LIKE 'image/%'`, opts)
}

func updateTableRegions(ctx context.Context, db *sql.DB, table string, extraWhere string, opts *RegionRecalcOptions) (int, error) {
	reg := Default()
	uid := appctx.UserIDFromCtx(ctx)
	query := fmt.Sprintf(
		`SELECT id, latitude, longitude FROM %s WHERE latitude IS NOT NULL AND longitude IS NOT NULL%s`,
		table, extraWhere,
	)
	args := []any{}
	if uid > 0 {
		query += " AND user_id = ?"
		args = append(args, uid)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("query %s for region update: %w", table, err)
	}
	defer rows.Close()

	type row struct {
		id        int64
		latitude  float64
		longitude float64
	}
	var items []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.latitude, &r.longitude); err != nil {
			return 0, fmt.Errorf("scan %s row for region update: %w", table, err)
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate %s for region update: %w", table, err)
	}
	if len(items) == 0 {
		return 0, nil
	}

	updateQuery := fmt.Sprintf(`UPDATE %s SET region = ?1 WHERE id = ?2`, table)
	updated := 0
	for i, item := range items {
		if opts != nil && opts.Cancel != nil && opts.Cancel() {
			return updated, ErrRegionRecalcCancelled
		}
		region := reg.RegionFromLatLng(item.latitude, item.longitude)
		if _, err := db.ExecContext(ctx, updateQuery, region, item.id); err != nil {
			return updated, fmt.Errorf("update %s id=%d region: %w", table, item.id, err)
		}
		updated++
		if opts != nil && opts.Progress != nil {
			opts.Progress(i+1, len(items))
		}
	}
	return updated, nil
}

package georegion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/daveontour/aimuseum/internal/appctx"
)

var ErrRegionRecalcCancelled = errors.New("region recalculation cancelled")
type regionDef struct {
	code string
	bbox [4]float64 // [min_lon, min_lat, max_lon, max_lat]
}

// regionBoxes lists bounding boxes in priority order (first match wins).
var regionBoxes = []regionDef{
	{code: "aus", bbox: [4]float64{110, -44, 152, -10}},
	{code: "dxb", bbox: [4]float64{54, 24, 56, 26}},
	{code: "ireland", bbox: [4]float64{-10.66, 51.38, -5.41, 55.43}},
	{code: "uk", bbox: [4]float64{-8.63, 49.86, 1.76, 60.86}},
	{code: "scandinavia", bbox: [4]float64{4.64, 54.56, 31.06, 71.19}},
	{code: "eur", bbox: [4]float64{-10, 35, 30, 70}},
	{code: "canada", bbox: [4]float64{-141.00, 41.90, -55.63, 71.96}},
	{code: "usa", bbox: [4]float64{-128, 20, -65, 50}},
	{code: "af", bbox: [4]float64{-20, -40, 50, 35}},
	{code: "me", bbox: [4]float64{30, 10, 60, 40}},
	{code: "malaysia", bbox: [4]float64{99.64, 0.85, 119.26, 7.36}},
	{code: "indonesia", bbox: [4]float64{95.01, -11.00, 141.01, 6.07}},
	{code: "india", bbox: [4]float64{68.11, 6.74, 97.41, 35.50}},
	{code: "thailand", bbox: [4]float64{97.34, 5.61, 105.63, 20.46}},
	{code: "asia", bbox: [4]float64{68, -12, 152, 54}},
	{code: "central_america", bbox: [4]float64{-116, 9, -76, 26}},
	{code: "carribean", bbox: [4]float64{-85, 12, -58, 25}},
	{code: "nz", bbox: [4]float64{163, -47, 179, -34}},
	{code: "south_america", bbox: [4]float64{-99, -56, -26, 12}},
}

func inRegionBBox(lat, lng float64, bbox [4]float64) bool {
	minLon, minLat, maxLon, maxLat := bbox[0], bbox[1], bbox[2], bbox[3]
	return lng >= minLon && lat >= minLat && lng <= maxLon && lat <= maxLat
}

// RegionFromLatLng maps coordinates to a world region code using bounding-box rules.
func RegionFromLatLng(lat, lng float64) string {
	for _, r := range regionBoxes {
		if inRegionBBox(lat, lng, r.bbox) {
			return r.code
		}
	}
	return "oth"
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
		region := RegionFromLatLng(item.latitude, item.longitude)
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
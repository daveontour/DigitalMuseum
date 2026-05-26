package georegion

import (
	"context"
	"database/sql"
	"fmt"
)

// RegionFromLatLng maps coordinates to a world region code using the same
// bounding-box rules as the legacy update_image_location_regions() function.
func RegionFromLatLng(lat, lng float64) string {
	switch {
	case lat >= -44 && lat <= -10 && lng >= 110 && lng <= 152:
		return "aus"
	case lat >= 24 && lat <= 26 && lng >= 54 && lng <= 56:
		return "dxb"
	case lat >= 35 && lat <= 70 && lng >= -10 && lng <= 30:
		return "eur"
	case lat >= 20 && lat <= 50 && lng >= -128 && lng <= -65:
		return "usa"
	case lat >= -40 && lat <= 35 && lng >= -20 && lng <= 50:
		return "af"
	case lat >= 10 && lat <= 40 && lng >= 30 && lng <= 60:
		return "me"
	case lat >= -12 && lat <= 54 && lng >= 68 && lng <= 152:
		return "asia"
	case lat >= 9 && lat <= 26 && lng >= -116 && lng <= -76:
		return "central_america"
	case lat >= 12 && lat <= 25 && lng >= -85 && lng <= -58:
		return "carribean"
	case lat >= -47 && lat <= -34 && lng >= 163 && lng <= 179:
		return "nz"
	case lat >= -56 && lat <= 12 && lng >= -99 && lng <= -26:
		return "south_america"
	default:
		return "oth"
	}
}

// UpdateMediaItemRegions sets media_items.region for rows with GPS coordinates.
func UpdateMediaItemRegions(ctx context.Context, db *sql.DB) error {
	return updateTableRegions(ctx, db, "media_items")
}

// UpdateLocationRegions sets locations.region for rows with GPS coordinates.
func UpdateLocationRegions(ctx context.Context, db *sql.DB) error {
	return updateTableRegions(ctx, db, "locations")
}

func updateTableRegions(ctx context.Context, db *sql.DB, table string) error {
	query := fmt.Sprintf(
		`SELECT id, latitude, longitude FROM %s WHERE latitude IS NOT NULL AND longitude IS NOT NULL`,
		table,
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query %s for region update: %w", table, err)
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
			return fmt.Errorf("scan %s row for region update: %w", table, err)
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s for region update: %w", table, err)
	}
	if len(items) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction for %s region update: %w", table, err)
	}
	defer tx.Rollback()

	updateQuery := fmt.Sprintf(`UPDATE %s SET region = ?1 WHERE id = ?2`, table)
	stmt, err := tx.PrepareContext(ctx, updateQuery)
	if err != nil {
		return fmt.Errorf("prepare %s region update: %w", table, err)
	}
	defer stmt.Close()

	for _, item := range items {
		region := RegionFromLatLng(item.latitude, item.longitude)
		if _, err := stmt.ExecContext(ctx, region, item.id); err != nil {
			return fmt.Errorf("update %s id=%d region: %w", table, item.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s region update: %w", table, err)
	}
	return nil
}

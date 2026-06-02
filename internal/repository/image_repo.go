package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ImageRepo runs queries against media_items, media_blobs, and facebook_albums tables.
type ImageRepo struct {
	pool *sql.DB
}

// NewImageRepo creates an ImageRepo backed by the given pool.
func NewImageRepo(pool *sql.DB) *ImageRepo {
	return &ImageRepo{pool: pool}
}

// ── media_items queries ───────────────────────────────────────────────────────

const mediaItemColumns = `
	id, media_blob_id, description, title, author, tags, categories, notes,
	available_for_task, media_type, processed, created_at, updated_at, embedding,
	year, month, day, latitude, longitude, altitude, rating, has_gps, google_maps_url,
	region, is_personal, is_business, is_social, is_promotional, is_spam,
	is_important, use_by_ai, is_referenced, source, source_reference`

// mediaItemColumnsQualified prefixes columns with mi. for use in JOINs with album_media (which also has id).
const mediaItemColumnsQualified = `
	mi.id, mi.media_blob_id, mi.description, mi.title, mi.author, mi.tags, mi.categories, mi.notes,
	mi.available_for_task, mi.media_type, mi.processed, mi.created_at, mi.updated_at, mi.embedding,
	mi.year, mi.month, mi.day, mi.latitude, mi.longitude, mi.altitude, mi.rating, mi.has_gps, mi.google_maps_url,
	mi.region, mi.is_personal, mi.is_business, mi.is_social, mi.is_promotional, mi.is_spam,
	mi.is_important, mi.use_by_ai, mi.is_referenced, mi.source, mi.source_reference`

// GetMediaItemByID returns a media_items row by primary key.
func (r *ImageRepo) GetMediaItemByID(ctx context.Context, id int64) (*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumns + ` FROM media_items WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	row := r.pool.QueryRowContext(ctx, q, args...)
	return scanMediaItem(row)
}

// GetBlobByID returns a media_blobs row by primary key.
func (r *ImageRepo) GetBlobByID(ctx context.Context, blobID int64) (*model.MediaBlob, error) {
	b := &model.MediaBlob{}
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, image_data, thumbnail_data FROM media_blobs WHERE id = ?1`, blobID,
	).Scan(&b.ID, &b.ImageData, &b.ThumbnailData)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBlobByID %d: %w", blobID, err)
	}
	return b, nil
}

// GetBlobByMetadataID returns the media_blobs row for a given media_items.id.
func (r *ImageRepo) GetBlobByMetadataID(ctx context.Context, metaID int64) (*model.MediaBlob, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT mb.id, mb.image_data, mb.thumbnail_data
		FROM media_blobs mb
		JOIN media_items mi ON mi.media_blob_id = mb.id
		WHERE mi.id = ?1`
	args := []any{metaID}
	q, args = addUIDFilterQualified(q, args, uid, "mi")
	b := &model.MediaBlob{}
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&b.ID, &b.ImageData, &b.ThumbnailData)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBlobByMetadataID %d: %w", metaID, err)
	}
	return b, nil
}

// GetMediaItemByBlobID returns the media_items row for a given media_blob.id.
func (r *ImageRepo) GetMediaItemByBlobID(ctx context.Context, blobID int64) (*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumns + ` FROM media_items WHERE media_blob_id = ?1`
	args := []any{blobID}
	q, args = addUIDFilter(q, args, uid)
	row := r.pool.QueryRowContext(ctx, q, args...)
	return scanMediaItem(row)
}

// Search returns media_items matching all provided filters (AND logic),
// always filtered to media_type LIKE 'image/%', ordered by created_at DESC.
func (r *ImageRepo) Search(ctx context.Context, p model.ImageSearchParams) ([]*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	var conds []string
	var args []any
	n := 1

	addLike := func(col, val string) {
		conds = append(conds, fmt.Sprintf("%s LIKE ?%d", col, n))
		args = append(args, "%"+val+"%")
		n++
	}
	addExact := func(col, val string) {
		conds = append(conds, fmt.Sprintf("%s LIKE ?%d", col, n))
		args = append(args, val) // no wildcards — mirrors Python .ilike(filters.source)
		n++
	}
	addEq := func(col string, val any) {
		conds = append(conds, fmt.Sprintf("%s = ?%d", col, n))
		args = append(args, val)
		n++
	}

	if p.Title != nil {
		addLike("title", *p.Title)
	}
	if p.Description != nil {
		addLike("description", *p.Description)
	}
	if p.Author != nil {
		addLike("author", *p.Author)
	}
	if p.Tags != nil {
		// Each comma-separated tag is OR'd: tag1 OR tag2 OR ...
		tagList := splitTrim(*p.Tags, ',')
		if len(tagList) > 0 {
			var orParts []string
			for _, tag := range tagList {
				orParts = append(orParts, fmt.Sprintf("tags LIKE ?%d", n))
				args = append(args, "%"+tag+"%")
				n++
			}
			conds = append(conds, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	if p.Categories != nil {
		addLike("categories", *p.Categories)
	}
	if p.Source != nil {
		addExact("source", *p.Source)
	}
	if p.SourceReference != nil {
		addLike("source_reference", *p.SourceReference)
	}
	if p.MediaType != nil {
		addLike("media_type", *p.MediaType)
	}
	if p.Region != nil {
		addLike("region", *p.Region)
	}
	if p.Year != nil {
		addEq("year", *p.Year)
	}
	if p.Month != nil {
		addEq("month", *p.Month)
	}
	if p.CreatedFrom != nil {
		conds = append(conds, fmt.Sprintf("date(created_at) >= date(?%d)", n))
		args = append(args, p.CreatedFrom.Format("2006-01-02"))
		n++
	}
	if p.CreatedTo != nil {
		conds = append(conds, fmt.Sprintf("date(created_at) <= date(?%d)", n))
		args = append(args, p.CreatedTo.Format("2006-01-02"))
		n++
	}
	if p.Rating != nil {
		addEq("rating", *p.Rating)
	} else {
		if p.RatingMin != nil {
			conds = append(conds, fmt.Sprintf("rating >= ?%d", n))
			args = append(args, *p.RatingMin)
			n++
		}
		if p.RatingMax != nil {
			conds = append(conds, fmt.Sprintf("rating <= ?%d", n))
			args = append(args, *p.RatingMax)
			n++
		}
	}
	if p.HasGPS != nil {
		addEq("has_gps", *p.HasGPS)
	}
	if p.HasThumbnail != nil && *p.HasThumbnail {
		conds = append(conds, `EXISTS (
			SELECT 1 FROM media_blobs mb
			WHERE mb.id = media_items.media_blob_id
			  AND mb.thumbnail_data IS NOT NULL
			  AND LENGTH(mb.thumbnail_data) > 0
		)`)
	}
	if p.AIClassified != nil && *p.AIClassified {
		conds = append(conds, "require_classification = FALSE")
	}
	if p.AIUnclassified != nil && *p.AIUnclassified {
		conds = append(conds, "require_classification = TRUE")
	}
	if p.AvailableForTask != nil {
		addEq("available_for_task", *p.AvailableForTask)
	}
	if p.Processed != nil {
		addEq("processed", *p.Processed)
	}

	// Always restrict to image/* media types
	conds = append(conds, "media_type LIKE 'image/%'")

	if uid > 0 {
		args = append(args, uid)
		conds = append(conds, fmt.Sprintf("user_id = ?%d", len(args)))
	}

	sql := `SELECT ` + mediaItemColumns + ` FROM media_items`
	if len(conds) > 0 {
		sql += " WHERE " + strings.Join(conds, " AND ")
	}
	sql += " ORDER BY created_at DESC"

	rows, err := r.pool.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("image Search: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// GetDistinctYears returns distinct non-null years ordered DESC.
func (r *ImageRepo) GetDistinctYears(ctx context.Context) ([]int, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT DISTINCT year FROM media_items WHERE year IS NOT NULL`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY year DESC"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetDistinctYears: %w", err)
	}
	defer rows.Close()

	var years []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

// GetAllTagStrings returns all non-empty tags column values (un-split).
// Caller is responsible for splitting and deduplicating.
func (r *ImageRepo) GetAllTagStrings(ctx context.Context) ([]string, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT tags FROM media_items WHERE tags IS NOT NULL AND tags != ''`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetAllTagStrings: %w", err)
	}
	defer rows.Close()

	var all []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		all = append(all, t)
	}
	return all, rows.Err()
}

// MediaTagEmbeddingRow is id + tags for tag-embedding backfill jobs.
type MediaTagEmbeddingRow struct {
	ID                    int64
	Tags                  *string
	RequireClassification bool
}

// ListMediaItemsForTagEmbeddingBackfill returns media_items rows that have non-empty tags (user-scoped).
// When onlyRequireClassification is true, only rows with require_classification=true are returned.
func (r *ImageRepo) ListMediaItemsForTagEmbeddingBackfill(ctx context.Context, onlyRequireClassification bool) ([]MediaTagEmbeddingRow, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, tags, require_classification FROM media_items WHERE tags IS NOT NULL AND TRIM(tags) != ''`
	if onlyRequireClassification {
		q += ` AND require_classification = TRUE`
	}
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY id ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListMediaItemsForTagEmbeddingBackfill: %w", err)
	}
	defer rows.Close()

	var out []MediaTagEmbeddingRow
	for rows.Next() {
		var row MediaTagEmbeddingRow
		if err := rows.Scan(&row.ID, &row.Tags, &row.RequireClassification); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// MediaClassificationQARow is one image flagged as classification-complete for tag-count QA.
type MediaClassificationQARow struct {
	ID   int64
	Tags *string
}

// ListMediaItemsForClassificationTagQA returns image items with require_classification=false
// so under-tagged rows can be re-queued for classification.
func (r *ImageRepo) ListMediaItemsForClassificationTagQA(ctx context.Context) ([]MediaClassificationQARow, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id, tags
		FROM media_items
		WHERE media_type LIKE 'image/%'
		  AND require_classification = FALSE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY id ASC`

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListMediaItemsForClassificationTagQA: %w", err)
	}
	defer rows.Close()

	var out []MediaClassificationQARow
	for rows.Next() {
		var row MediaClassificationQARow
		if err := rows.Scan(&row.ID, &row.Tags); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListImageIDsRequireClassification returns image media_items IDs flagged for
// classification (require_classification=true).
func (r *ImageRepo) ListImageIDsRequireClassification(ctx context.Context) ([]int64, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id
		FROM media_items
		WHERE media_type LIKE 'image/%'
		  AND require_classification = TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id ASC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListImageIDsRequireClassification: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListImageIDsMissingTag returns image media_items IDs that do not contain the given tag.
// Matching is case-insensitive and treats NULL/empty tags as missing.
func (r *ImageRepo) ListImageIDsMissingTag(ctx context.Context, tag string) ([]int64, error) {
	uid := uidFromCtx(ctx)
	needle := strings.TrimSpace(tag)
	if needle == "" {
		return nil, fmt.Errorf("tag is required")
	}
	q := `
		SELECT id
		FROM media_items
		WHERE media_type LIKE 'image/%'
		  AND (tags IS NULL OR TRIM(tags) = '' OR LOWER(tags) NOT LIKE LOWER(?1))`
	args := []any{"%" + needle + "%"}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id ASC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListImageIDsMissingTag: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetLocations returns media_items with GPS data (has_gps=true or lat/lng non-null).
func (r *ImageRepo) GetLocations(ctx context.Context) ([]*model.MediaItem, error) {
	// uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumns + ` FROM media_items
	      WHERE has_gps = TRUE
	         OR (latitude IS NOT NULL AND longitude IS NOT NULL)`
	args := []any{}
	// q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetLocations: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

var knownLocationCategorySources = []string{
	"filesystem", "biography", "facebook_album", "whatsapp",
	"email_attachment", "gmail_attachment",
	"message", "imessage", "sms", "message_attachment",
}

const locationGPSPredicate = `(has_gps != 0 OR (latitude IS NOT NULL AND longitude IS NOT NULL))`

func resolveLocationCategorySources(categories []string) (sources []string, includeOther bool) {
	if len(categories) > 0 && strings.ToLower(strings.TrimSpace(categories[0])) == "all" {
		return append([]string{}, knownLocationCategorySources...), true
	}

	seen := make(map[string]struct{})
	add := func(source string) {
		source = strings.ToLower(strings.TrimSpace(source))
		if source == "" {
			return
		}
		if _, ok := seen[source]; ok {
			return
		}
		seen[source] = struct{}{}
		sources = append(sources, source)
	}

	for _, cat := range categories {
		switch strings.ToLower(strings.TrimSpace(cat)) {
		case "filesystem":
			add("filesystem")
		case "biography":
			add("biography")
		case "whatsapp":
			add("whatsapp")
		case "email":
			add("email_attachment")
			add("gmail_attachment")
		case "message":
			add("message")
			add("imessage")
			add("sms")
			add("message_attachment")
		case "facebook":
			add("facebook_album")
		case "other":
			includeOther = true
		}
	}
	return sources, includeOther
}

func buildRandomLocationSourceFilter(sources []string, includeOther bool, start int) (sql string, args []any, next int) {
	var parts []string
	n := start

	if len(sources) > 0 {
		inCond, inArgs, nextN := sqlutil.StringIN(`LOWER(TRIM(COALESCE(source, '')))`, sources, n)
		parts = append(parts, inCond)
		args = append(args, inArgs...)
		n = nextN
	}

	// if includeOther {
	// 	ph := make([]string, len(knownLocationCategorySources))
	// 	for i, src := range knownLocationCategorySources {
	// 		ph[i] = fmt.Sprintf("?%d", n)
	// 		args = append(args, src)
	// 		n++
	// 	}
	// 	parts = append(parts, fmt.Sprintf(
	// 		`(TRIM(COALESCE(source, '')) = '' OR LOWER(TRIM(COALESCE(source, ''))) NOT IN (%s))`,
	// 		strings.Join(ph, ", "),
	// 	))
	// }

	if len(parts) == 0 {
		return "", nil, start
	}
	return "(" + strings.Join(parts, " OR ") + ")", args, n
}

// GetRandomLocationsByCategories returns up to limit random media_items with GPS for the given map categories.
func (r *ImageRepo) GetRandomLocationsByCategories(ctx context.Context, categories []string, region string, limit int) ([]*model.MediaItem, error) {
	sources, includeOther := resolveLocationCategorySources(categories)
	sourceSQL, sourceArgs, nextN := buildRandomLocationSourceFilter(sources, includeOther, 1)
	if sourceSQL == "" {
		return []*model.MediaItem{}, nil
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}

	args := append([]any{}, sourceArgs...)
	whereSQL := sourceSQL
	if region = strings.TrimSpace(region); region != "" {
		whereSQL += fmt.Sprintf(" AND region = ?%d", nextN)
		args = append(args, region)
		nextN++
	}
	limitIdx := len(args) + 1
	args = append(args, limit)
	q := fmt.Sprintf(`SELECT %s FROM media_items
	      WHERE id IN (
	        SELECT id FROM media_items
	        WHERE latitude IS NOT NULL AND longitude IS NOT NULL
	          AND %s
	        ORDER BY RANDOM()
	        LIMIT ?%d
	      )`, mediaItemColumns, whereSQL, limitIdx)

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetRandomLocationsByCategories: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// NearbyLocationRow is a media item with its distance from a search center (km).
type NearbyLocationRow struct {
	Item       *model.MediaItem
	DistanceKm float64
}

// GetNearbyLocations returns GPS-tagged media_items within radiusKm of lat/lng, ordered by distance.
func (r *ImageRepo) GetNearbyLocations(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]NearbyLocationRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}

	latRad := lat * math.Pi / 180.0
	latDelta := radiusKm / 111.0
	lngDelta := radiusKm / 111.0
	if cosLat := math.Cos(latRad); cosLat > 1e-6 {
		lngDelta = radiusKm / (111.0 * cosLat)
	}
	if lngDelta > 180 {
		lngDelta = 180
	}
	latMin := lat - latDelta
	latMax := lat + latDelta
	lngMin := lng - lngDelta
	lngMax := lng + lngDelta

	// Bounding-box prefilter in SQL; Haversine distance computed in Go because
	// stock SQLite (go-sqlite3) does not expose acos/cos/sin.
	q := `SELECT ` + mediaItemColumns + ` FROM media_items
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL
		  AND latitude BETWEEN ?1 AND ?2
		  AND longitude BETWEEN ?3 AND ?4`

	rows, err := r.pool.QueryContext(ctx, q, latMin, latMax, lngMin, lngMax)
	if err != nil {
		return nil, fmt.Errorf("GetNearbyLocations: %w", err)
	}
	defer rows.Close()

	items, err := scanMediaItems(rows)
	if err != nil {
		return nil, err
	}

	var results []NearbyLocationRow
	for _, item := range items {
		if item.Latitude == nil || item.Longitude == nil {
			continue
		}
		distanceKm := haversineKm(lat, lng, *item.Latitude, *item.Longitude)
		if distanceKm > radiusKm {
			continue
		}
		results = append(results, NearbyLocationRow{Item: item, DistanceKm: distanceKm})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DistanceKm < results[j].DistanceKm
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// CountGPSBySource returns counts of media_items with GPS data grouped by source.
func (r *ImageRepo) CountGPSBySource(ctx context.Context) ([]model.GPSCountBySource, error) {
	//uid := uidFromCtx(ctx)
	q := `SELECT COALESCE(NULLIF(TRIM(source), ''), ''), COUNT(*)
	      FROM media_items
	      WHERE has_gps = TRUE
	         OR (latitude IS NOT NULL AND longitude IS NOT NULL)`
	args := []any{}
	//q, args = addUIDFilter(q, args, uid)
	q += ` GROUP BY 1 ORDER BY 2 DESC, 1 ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("CountGPSBySource: %w", err)
	}
	defer rows.Close()

	var counts []model.GPSCountBySource
	for rows.Next() {
		var row model.GPSCountBySource
		if err := rows.Scan(&row.Source, &row.Count); err != nil {
			return nil, err
		}
		counts = append(counts, row)
	}
	return counts, rows.Err()
}

// CountImagesByRegion returns counts of image media_items grouped by region code.
func (r *ImageRepo) CountImagesByRegion(ctx context.Context) ([]model.ImageRegionCount, error) {
	q := `SELECT TRIM(region), COUNT(*)
	      FROM media_items
	      WHERE media_type LIKE 'image/%'
	        AND region IS NOT NULL AND TRIM(region) != ''`
	args := []any{}
	// uid := uidFromCtx(ctx)
	// q, args = addUIDFilter(q, args, uid)
	q += ` GROUP BY TRIM(region) ORDER BY COUNT(*) DESC, TRIM(region) ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("CountImagesByRegion: %w", err)
	}
	defer rows.Close()

	var counts []model.ImageRegionCount
	for rows.Next() {
		var row model.ImageRegionCount
		if err := rows.Scan(&row.Region, &row.Count); err != nil {
			return nil, err
		}
		counts = append(counts, row)
	}
	return counts, rows.Err()
}

// CountGPSByRegion returns counts of GPS-tagged media_items grouped by region code.
func (r *ImageRepo) CountGPSByRegion(ctx context.Context) ([]model.GPSRegionCount, error) {
	q := `SELECT TRIM(region), COUNT(*)
	      FROM media_items
	      WHERE latitude IS NOT NULL AND longitude IS NOT NULL
	        AND region IS NOT NULL AND TRIM(region) != ''`
	args := []any{}
	// uid := uidFromCtx(ctx)
	// q, args = addUIDFilter(q, args, uid)
	q += ` GROUP BY TRIM(region) ORDER BY COUNT(*) DESC, TRIM(region) ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("CountGPSByRegion: %w", err)
	}
	defer rows.Close()

	var counts []model.GPSRegionCount
	for rows.Next() {
		var row model.GPSRegionCount
		if err := rows.Scan(&row.Region, &row.Count); err != nil {
			return nil, err
		}
		counts = append(counts, row)
	}
	return counts, rows.Err()
}

// GetFacebookPlaces returns all locations with source='facebook'.
func (r *ImageRepo) GetFacebookPlaces(ctx context.Context) ([]model.FacebookPlaceItem, error) {
	rows, err := r.pool.QueryContext(ctx, `
		SELECT id, name, description, address, latitude, longitude, region, source_reference
		FROM locations
		WHERE source = 'facebook'
		ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("GetFacebookPlaces: %w", err)
	}
	defer rows.Close()

	var places []model.FacebookPlaceItem
	for rows.Next() {
		var p model.FacebookPlaceItem
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Address, &p.Latitude, &p.Longitude, &p.Region, &p.SourceReference); err != nil {
			return nil, fmt.Errorf("GetFacebookPlaces scan: %w", err)
		}
		places = append(places, p)
	}
	return places, rows.Err()
}

// ── Write / delete ────────────────────────────────────────────────────────────

// CountCommaSeparatedTags returns the number of non-empty comma-separated tag tokens.
func CountCommaSeparatedTags(raw string) int {
	return len(commaSplitTags(raw))
}

// commaSplitTags splits comma-separated tags into trimmed, non-empty parts.
func commaSplitTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// tagSetLowercase builds a case-insensitive set from one or more comma-separated tag strings.
func tagSetLowercase(tagStrings ...string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, raw := range tagStrings {
		for _, t := range commaSplitTags(raw) {
			set[strings.ToLower(t)] = struct{}{}
		}
	}
	return set
}

// filterTagsNotAlreadyPresent returns tags from newTags that are not already in existingTags
// (case-insensitive). Duplicates within newTags are collapsed, preserving first-seen casing.
func filterTagsNotAlreadyPresent(existingTags *string, newTags string) []string {
	existing := ""
	if existingTags != nil {
		existing = *existingTags
	}
	present := tagSetLowercase(existing)
	var additions []string
	for _, t := range commaSplitTags(newTags) {
		key := strings.ToLower(t)
		if _, ok := present[key]; ok {
			continue
		}
		present[key] = struct{}{}
		additions = append(additions, t)
	}
	return additions
}

// UpdateTags updates the tags column for a media item (merge: append to existing).
func (r *ImageRepo) UpdateTags(ctx context.Context, id int64, newTags string) (bool, error) {
	uid := uidFromCtx(ctx)
	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Debug("UpdateTags trace: start",
		"media_item_id", id,
		"user_id", uid,
		"new_tags_len", len(newTags),
		"new_tags_preview", truncateForLog(newTags, 120),
	)

	q := `SELECT tags FROM media_items WHERE id = ?1`
	args := []any{id}
	//q, args = addUIDFilter(q, args, uid)
	var existing *string
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&existing)
	if err != nil {
		if isNoRows(err) {
			slog.Warn("UpdateTags trace: SELECT found no row (skipping UPDATE)",
				"media_item_id", id,
				"user_id", uid,
				"select_err", "sql.ErrNoRows",
			)
			return false, nil
		}
		slog.Warn("UpdateTags trace: SELECT failed",
			"media_item_id", id,
			"user_id", uid,
			"err", err,
		)
		return false, fmt.Errorf("UpdateTags: %w", err)
	}

	existingLen := 0
	existingPreview := ""
	if existing != nil {
		existingLen = len(*existing)
		existingPreview = truncateForLog(*existing, 120)
	}
	slog.Debug("UpdateTags trace: after SELECT",
		"media_item_id", id,
		"user_id", uid,
		"existing_is_nil", existing == nil,
		"existing_len", existingLen,
		"existing_preview", existingPreview,
	)

	additions := filterTagsNotAlreadyPresent(existing, newTags)
	if len(additions) == 0 {
		slog.Debug("UpdateTags trace: no new tags to append (all already present)",
			"media_item_id", id,
			"user_id", uid,
		)
		return true, nil
	}
	merged := strings.Join(additions, ", ")
	if existing != nil && strings.TrimSpace(*existing) != "" {
		merged = strings.TrimSpace(*existing) + ", " + merged
	}
	uq := `UPDATE media_items SET tags = ?2, require_classification = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = ?1`
	uargs := []any{id, merged}
	//uq, uargs = addUIDFilter(uq, uargs, uid)

	slog.Debug("UpdateTags trace: before UPDATE",
		"media_item_id", id,
		"user_id", uid,
		"merged_len", len(merged),
		"merged_preview", truncateForLog(merged, 200),
	)

	res, err := r.pool.ExecContext(ctx, uq, uargs...)
	if err != nil {
		slog.Warn("UpdateTags trace: UPDATE ExecContext failed",
			"media_item_id", id,
			"user_id", uid,
			"err", err,
		)
		return false, fmt.Errorf("UpdateTags: %w", err)
	}

	n, raErr := res.RowsAffected()
	if raErr != nil {
		slog.Warn("UpdateTags trace: RowsAffected returned error (treating as 0)",
			"media_item_id", id,
			"user_id", uid,
			"rows_affected_err", raErr,
		)
		n = 0
	}
	slog.Debug("UpdateTags trace: after UPDATE",
		"media_item_id", id,
		"user_id", uid,
		"rows_affected", n,
		"rows_affected_ok", raErr == nil,
	)
	if n == 0 {
		slog.Warn("UpdateTags trace: UPDATE reported 0 rows changed",
			"media_item_id", id,
			"user_id", uid,
			"merged_len", len(merged),
			"merged_preview", truncateForLog(merged, 200),
			"note", "id may not exist in media_items, value unchanged (SQLite no-op), or driver did not report changes",
		)
	}

	return n > 0, nil
}

// UpdateMetadata updates description, tags, and/or rating for a media item.
func (r *ImageRepo) UpdateMetadata(ctx context.Context, id int64, description, tags *string, rating *int) (bool, error) {
	item, err := r.GetMediaItemByID(ctx, id)
	if err != nil || item == nil {
		return false, nil
	}
	//uid := uidFromCtx(ctx)
	var setParts []string
	var args []any
	n := 1
	args = append(args, id)
	n++
	if description != nil {
		setParts = append(setParts, fmt.Sprintf("description = ?%d", n))
		args = append(args, *description)
		n++
	}
	if tags != nil {
		setParts = append(setParts, fmt.Sprintf("tags = ?%d", n))
		args = append(args, *tags)
		n++
		setParts = append(setParts, "require_classification = TRUE")
	}
	if rating != nil {
		setParts = append(setParts, fmt.Sprintf("rating = ?%d", n))
		args = append(args, *rating)
		n++
	}
	if len(setParts) == 0 {
		return true, nil
	}
	setParts = append(setParts, "updated_at = CURRENT_TIMESTAMP")
	sql := fmt.Sprintf(`UPDATE media_items SET %s WHERE id = ?1`, strings.Join(setParts, ", "))
	//sql, args = addUIDFilterDollar(sql, args, uid)
	res, err := r.pool.ExecContext(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("UpdateMetadata: %w", err)
	}
	return rowsAffectedOrZero(res) > 0, nil
}

// SetRequireClassification sets media_items.require_classification for one item.
func (r *ImageRepo) SetRequireClassification(ctx context.Context, id int64, require bool) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE media_items
	      SET require_classification = ?2, updated_at = CURRENT_TIMESTAMP
	      WHERE id = ?1`
	args := []any{id, require}
	q, args = addUIDFilterDollar(q, args, uid)
	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("SetRequireClassification: %w", err)
	}
	return rowsAffectedOrZero(res) > 0, nil
}

// DeleteByMetadataID deletes a media_items row and its media_blobs row.
func (r *ImageRepo) DeleteByMetadataID(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT media_blob_id FROM media_items WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	var blobID int64
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&blobID)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("DeleteByMetadataID: %w", err)
	}
	_, _ = r.pool.ExecContext(ctx, `DELETE FROM media_tag_embeddings WHERE rowid = ?1`, id)
	dq := `DELETE FROM media_items WHERE id = ?1`
	dargs := []any{id}
	dq, dargs = addUIDFilter(dq, dargs, uid)
	_, err = r.pool.ExecContext(ctx, dq, dargs...)
	if err != nil {
		return false, fmt.Errorf("DeleteByMetadataID: %w", err)
	}
	_, _ = r.pool.ExecContext(ctx, `DELETE FROM media_blobs WHERE id = ?1`, blobID)
	return true, nil
}

// DeleteByIDRange deletes media_items (and associated blobs) matching the criteria.
// If all is true, deletes all image media_items. If startID/endID are set, deletes in range.
func (r *ImageRepo) DeleteByIDRange(ctx context.Context, all bool, startID, endID *int64) (int64, error) {
	if all && (startID != nil || endID != nil) {
		return 0, fmt.Errorf("cannot specify both all and ID range")
	}
	if !all && startID == nil && endID == nil {
		return 0, fmt.Errorf("must specify either all=true or start_id/end_id")
	}
	if startID != nil && endID != nil && *startID > *endID {
		return 0, fmt.Errorf("start_id must be <= end_id")
	}

	uid := uidFromCtx(ctx)
	var cond string
	var args []any
	n := 1
	if all {
		cond = "media_type LIKE 'image/%'"
	} else {
		parts := []string{"media_type LIKE 'image/%'"}
		if startID != nil {
			parts = append(parts, fmt.Sprintf("id >= ?%d", n))
			args = append(args, *startID)
			n++
		}
		if endID != nil {
			parts = append(parts, fmt.Sprintf("id <= ?%d", n))
			args = append(args, *endID)
			n++
		}
		cond = strings.Join(parts, " AND ")
	}
	q := `SELECT id, media_blob_id FROM media_items WHERE ` + cond
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("DeleteByIDRange: %w", err)
	}
	defer rows.Close()
	var ids, blobIDs []int64
	for rows.Next() {
		var id, blobID int64
		if err := rows.Scan(&id, &blobID); err != nil {
			return 0, err
		}
		ids = append(ids, id)
		blobIDs = append(blobIDs, blobID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	for _, mid := range ids {
		_, _ = r.pool.ExecContext(ctx, `DELETE FROM media_tag_embeddings WHERE rowid = ?`, mid)
	}
	miCond, miArgs, _ := sqlutil.Int64IN("id", ids, 1)
	delMI := fmt.Sprintf(`DELETE FROM media_items WHERE %s`, miCond)
	_, err = r.pool.ExecContext(ctx, delMI, miArgs...)
	if err != nil {
		return 0, fmt.Errorf("DeleteByIDRange: %w", err)
	}
	mbCond, mbArgs, _ := sqlutil.Int64IN("id", blobIDs, 1)
	delMB := fmt.Sprintf(`DELETE FROM media_blobs WHERE %s`, mbCond)
	_, err = r.pool.ExecContext(ctx, delMB, mbArgs...)
	if err != nil {
		return 0, fmt.Errorf("DeleteByIDRange: %w", err)
	}
	return int64(len(ids)), nil
}

// ReferencedItem holds id, blob id, and source path for reference import.
type ReferencedItem struct {
	ID              int64
	MediaBlobID     int64
	SourceReference string
}

// ListReferencedItems returns media_items where is_referenced=true and source_reference is not empty.
func (r *ImageRepo) ListReferencedItems(ctx context.Context) ([]ReferencedItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, media_blob_id, COALESCE(source_reference,'') FROM media_items
	      WHERE is_referenced = TRUE AND source_reference IS NOT NULL AND source_reference != ''`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListReferencedItems: %w", err)
	}
	defer rows.Close()
	var items []ReferencedItem
	for rows.Next() {
		var it ReferencedItem
		if err := rows.Scan(&it.ID, &it.MediaBlobID, &it.SourceReference); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// UpdateBlobImageDataAndClearReferenced updates media_blobs.image_data and sets media_items.is_referenced=false
// in a single transaction so both updates are atomic.
func (r *ImageRepo) UpdateBlobImageDataAndClearReferenced(ctx context.Context, itemID, blobID int64, data []byte) error {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("UpdateBlobImageDataAndClearReferenced begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx, `UPDATE media_blobs SET image_data = ?2 WHERE id = ?1`, blobID, data)
	if err != nil {
		return fmt.Errorf("UpdateBlobImageData: %w", err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE media_items SET is_referenced = FALSE, updated_at = CURRENT_TIMESTAMP WHERE id = ?1`, itemID)
	if err != nil {
		return fmt.Errorf("SetItemNotReferenced: %w", err)
	}
	return tx.Commit()
}

// ExportItem holds fields needed for image export.
type ExportItem struct {
	ID          int64
	MediaBlobID int64
	Title       *string
	MediaType   *string
	SourceRef   *string
}

// ListMediaItemsForExport returns all media_items (id, blob_id, title, media_type, source_reference).
func (r *ImageRepo) ListMediaItemsForExport(ctx context.Context) ([]ExportItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, media_blob_id, title, media_type, source_reference FROM media_items WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListMediaItemsForExport: %w", err)
	}
	defer rows.Close()
	var items []ExportItem
	for rows.Next() {
		var it ExportItem
		if err := rows.Scan(&it.ID, &it.MediaBlobID, &it.Title, &it.MediaType, &it.SourceRef); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// GetBlobImageData returns image_data for a blob. Returns nil if not found or empty.
func (r *ImageRepo) GetBlobImageData(ctx context.Context, blobID int64) ([]byte, error) {
	var data []byte
	err := r.pool.QueryRowContext(ctx, `SELECT image_data FROM media_blobs WHERE id = ?1`, blobID).Scan(&data)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// ── facebook_albums queries ───────────────────────────────────────────────────

// GetFacebookAlbums returns all albums with their image count.
func (r *ImageRepo) GetFacebookAlbums(ctx context.Context) ([]model.FacebookAlbumResponse, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT fa.id, fa.name, fa.description, fa.cover_photo_uri,
		       COUNT(DISTINCT am.id) AS image_count
		FROM facebook_albums fa
		LEFT JOIN album_media am ON fa.id = am.album_id
		WHERE TRUE`
	args := []any{}
	q, args = addUIDFilterQualified(q, args, uid, "fa")
	q += `
		GROUP BY fa.id, fa.name, fa.description, fa.cover_photo_uri
		ORDER BY fa.name`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetFacebookAlbums: %w", err)
	}
	defer rows.Close()

	var albums []model.FacebookAlbumResponse
	for rows.Next() {
		var a model.FacebookAlbumResponse
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.CoverPhotoURI, &a.ImageCount); err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// GetAlbumImages returns media_items linked to an album, ordered by created_at ASC.
func (r *ImageRepo) GetAlbumImages(ctx context.Context, albumID int64) ([]*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumnsQualified + `
	      FROM media_items mi
	      JOIN album_media am ON mi.id = am.media_item_id
	      WHERE am.album_id = ?1`
	args := []any{albumID}
	q, args = addUIDFilterQualified(q, args, uid, "mi")
	q += ` ORDER BY mi.created_at ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetAlbumImages: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// GetAlbumImageByID returns a media_items row that is linked to any album.
func (r *ImageRepo) GetAlbumImageByID(ctx context.Context, imageID int64) (*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumnsQualified + `
	      FROM media_items mi
	      JOIN album_media am ON mi.id = am.media_item_id
	      WHERE mi.id = ?1`
	args := []any{imageID}
	q, args = addUIDFilterQualified(q, args, uid, "mi")
	q += ` LIMIT 1`
	row := r.pool.QueryRowContext(ctx, q, args...)
	return scanMediaItem(row)
}

// GetFacebookPostsParams holds optional filters for GetFacebookPosts.
type GetFacebookPostsParams struct {
	Search   string  // LIKE on post_text and title
	PostIDs  []int64 // filter to these IDs
	Page     int     // 1-based
	PageSize int     // 1-200
}

// GetFacebookPosts returns paginated Facebook posts with media count.
func (r *ImageRepo) GetFacebookPosts(ctx context.Context, p GetFacebookPostsParams) (*model.FacebookPostsResponse, error) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 50
	}
	if p.PageSize > 200 {
		p.PageSize = 200
	}

	uid := uidFromCtx(ctx)

	baseQuery := `
		SELECT fp.id, fp.timestamp, fp.title, fp.post_text, fp.external_url, fp.post_type,
		       CAST(COUNT(DISTINCT pm.id) AS INTEGER) AS media_count
		FROM facebook_posts fp
		LEFT JOIN post_media pm ON fp.id = pm.post_id
		WHERE TRUE
	`

	var args []any
	var conds []string
	argNum := 1

	if p.Search != "" {
		conds = append(conds, fmt.Sprintf("(fp.post_text LIKE ?%d OR fp.title LIKE ?%d)", argNum, argNum))
		args = append(args, "%"+p.Search+"%")
		argNum++
	}
	if len(p.PostIDs) > 0 {
		inCond, inArgs, next := sqlutil.Int64IN("fp.id", p.PostIDs, argNum)
		conds = append(conds, inCond)
		args = append(args, inArgs...)
		argNum = next
	}

	if len(conds) > 0 {
		baseQuery += " AND " + strings.Join(conds, " AND ")
	}

	if uid > 0 {
		args = append(args, uid)
		baseQuery += fmt.Sprintf(" AND fp.user_id = ?%d", len(args))
		argNum++
	}

	baseQuery += " GROUP BY fp.id, fp.timestamp, fp.title, fp.post_text, fp.external_url, fp.post_type"
	baseQuery += " ORDER BY " + sqlutil.OrderByDescNullsLast("fp.timestamp")

	// Count total
	countQuery := `SELECT COUNT(*) FROM (` + baseQuery + `) AS sub`
	var total int
	if err := r.pool.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("GetFacebookPosts count: %w", err)
	}

	// Fetch page
	offset := (p.Page - 1) * p.PageSize
	args = append(args, p.PageSize, offset)
	pageQuery := baseQuery + fmt.Sprintf(" LIMIT ?%d OFFSET ?%d", len(args)-1, len(args))

	rows, err := r.pool.QueryContext(ctx, pageQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("GetFacebookPosts: %w", err)
	}
	defer rows.Close()

	var posts []model.FacebookPostListItem
	for rows.Next() {
		var p model.FacebookPostListItem
		if err := rows.Scan(&p.ID, &p.Timestamp, &p.Title, &p.PostText, &p.ExternalURL, &p.PostType, &p.MediaCount); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if posts == nil {
		posts = []model.FacebookPostListItem{}
	}

	return &model.FacebookPostsResponse{
		Total:    total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Posts:    posts,
	}, nil
}

// GetPostMediaByID returns a media_items row that is linked to a Facebook post via post_media.
func (r *ImageRepo) GetPostMediaByID(ctx context.Context, mediaID int64) (*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumnsQualified + `
	      FROM media_items mi
	      JOIN post_media pm ON mi.id = pm.media_item_id
	      WHERE mi.id = ?1`
	args := []any{mediaID}
	q, args = addUIDFilterQualified(q, args, uid, "mi")
	q += ` LIMIT 1`
	row := r.pool.QueryRowContext(ctx, q, args...)
	return scanMediaItem(row)
}

// GetPostMedia returns media_items linked to a Facebook post via post_media, ordered by created_at ASC.
func (r *ImageRepo) GetPostMedia(ctx context.Context, postID int64) ([]*model.MediaItem, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + mediaItemColumnsQualified + `
	      FROM media_items mi
	      JOIN post_media pm ON mi.id = pm.media_item_id
	      WHERE pm.post_id = ?1`
	args := []any{postID}
	q, args = addUIDFilterQualified(q, args, uid, "mi")
	q += ` ORDER BY mi.created_at ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("GetPostMedia: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// ── scanners ──────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanMediaItem(row scanner) (*model.MediaItem, error) {
	m := &model.MediaItem{}
	err := row.Scan(
		&m.ID, &m.MediaBlobID, &m.Description, &m.Title, &m.Author, &m.Tags,
		&m.Categories, &m.Notes, &m.AvailableForTask, &m.MediaType, &m.Processed,
		&m.CreatedAt, &m.UpdatedAt, &m.Embedding,
		&m.Year, &m.Month, &m.Day, &m.Latitude, &m.Longitude, &m.Altitude,
		&m.Rating, &m.HasGPS, &m.GoogleMapsURL, &m.Region,
		&m.IsPersonal, &m.IsBusiness, &m.IsSocial, &m.IsPromotional,
		&m.IsSpam, &m.IsImportant, &m.UseByAI, &m.IsReferenced,
		&m.Source, &m.SourceReference,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func scanMediaItems(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*model.MediaItem, error) {
	var items []*model.MediaItem
	for rows.Next() {
		m := &model.MediaItem{}
		if err := rows.Scan(
			&m.ID, &m.MediaBlobID, &m.Description, &m.Title, &m.Author, &m.Tags,
			&m.Categories, &m.Notes, &m.AvailableForTask, &m.MediaType, &m.Processed,
			&m.CreatedAt, &m.UpdatedAt, &m.Embedding,
			&m.Year, &m.Month, &m.Day, &m.Latitude, &m.Longitude, &m.Altitude,
			&m.Rating, &m.HasGPS, &m.GoogleMapsURL, &m.Region,
			&m.IsPersonal, &m.IsBusiness, &m.IsSocial, &m.IsPromotional,
			&m.IsSpam, &m.IsImportant, &m.UseByAI, &m.IsReferenced,
			&m.Source, &m.SourceReference,
		); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// truncateForLog returns s truncated to at most maxRunes runes (for slog fields).
func truncateForLog(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

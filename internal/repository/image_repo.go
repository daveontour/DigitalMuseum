package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ImageRepo runs queries against media_items, media_blobs, and facebook_albums tables.
type ImageRepo struct {
	pool *pgxpool.Pool
}

// NewImageRepo creates an ImageRepo backed by the given pool.
func NewImageRepo(pool *pgxpool.Pool) *ImageRepo {
	return &ImageRepo{pool: pool}
}

// ── media_items queries ───────────────────────────────────────────────────────

const mediaItemColumns = `
	id, media_blob_id, description, title, author, tags, categories, notes,
	available_for_task, media_type, processed, created_at, updated_at, embedding,
	year, month, latitude, longitude, altitude, rating, has_gps, google_maps_url,
	region, is_personal, is_business, is_social, is_promotional, is_spam,
	is_important, use_by_ai, is_referenced, source, source_reference`

// mediaItemColumnsQualified prefixes columns with mi. for use in JOINs with album_media (which also has id).
const mediaItemColumnsQualified = `
	mi.id, mi.media_blob_id, mi.description, mi.title, mi.author, mi.tags, mi.categories, mi.notes,
	mi.available_for_task, mi.media_type, mi.processed, mi.created_at, mi.updated_at, mi.embedding,
	mi.year, mi.month, mi.latitude, mi.longitude, mi.altitude, mi.rating, mi.has_gps, mi.google_maps_url,
	mi.region, mi.is_personal, mi.is_business, mi.is_social, mi.is_promotional, mi.is_spam,
	mi.is_important, mi.use_by_ai, mi.is_referenced, mi.source, mi.source_reference`

// GetMediaItemByID returns a media_items row by primary key.
func (r *ImageRepo) GetMediaItemByID(ctx context.Context, id int64) (*model.MediaItem, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = $1`, id)
	return scanMediaItem(row)
}

// GetBlobByID returns a media_blobs row by primary key.
func (r *ImageRepo) GetBlobByID(ctx context.Context, blobID int64) (*model.MediaBlob, error) {
	b := &model.MediaBlob{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, image_data, thumbnail_data FROM media_blobs WHERE id = $1`, blobID,
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
	b := &model.MediaBlob{}
	err := r.pool.QueryRow(ctx, `
		SELECT mb.id, mb.image_data, mb.thumbnail_data
		FROM media_blobs mb
		JOIN media_items mi ON mi.media_blob_id = mb.id
		WHERE mi.id = $1`, metaID,
	).Scan(&b.ID, &b.ImageData, &b.ThumbnailData)
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
	row := r.pool.QueryRow(ctx,
		`SELECT `+mediaItemColumns+` FROM media_items WHERE media_blob_id = $1`, blobID)
	return scanMediaItem(row)
}

// Search returns media_items matching all provided filters (AND logic),
// always filtered to media_type LIKE 'image/%', ordered by created_at DESC.
func (r *ImageRepo) Search(ctx context.Context, p model.ImageSearchParams) ([]*model.MediaItem, error) {
	var conds []string
	var args []any
	n := 1

	addLike := func(col, val string) {
		conds = append(conds, fmt.Sprintf("%s ILIKE $%d", col, n))
		args = append(args, "%"+val+"%")
		n++
	}
	addExact := func(col, val string) {
		conds = append(conds, fmt.Sprintf("%s ILIKE $%d", col, n))
		args = append(args, val) // no wildcards — mirrors Python .ilike(filters.source)
		n++
	}
	addEq := func(col string, val any) {
		conds = append(conds, fmt.Sprintf("%s = $%d", col, n))
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
				orParts = append(orParts, fmt.Sprintf("tags ILIKE $%d", n))
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
	if p.Rating != nil {
		addEq("rating", *p.Rating)
	} else {
		if p.RatingMin != nil {
			conds = append(conds, fmt.Sprintf("rating >= $%d", n))
			args = append(args, *p.RatingMin)
			n++
		}
		if p.RatingMax != nil {
			conds = append(conds, fmt.Sprintf("rating <= $%d", n))
			args = append(args, *p.RatingMax)
			n++
		}
	}
	if p.HasGPS != nil {
		addEq("has_gps", *p.HasGPS)
	}
	if p.AvailableForTask != nil {
		addEq("available_for_task", *p.AvailableForTask)
	}
	if p.Processed != nil {
		addEq("processed", *p.Processed)
	}

	// Always restrict to image/* media types
	conds = append(conds, "media_type LIKE 'image/%'")

	sql := `SELECT ` + mediaItemColumns + ` FROM media_items`
	if len(conds) > 0 {
		sql += " WHERE " + strings.Join(conds, " AND ")
	}
	sql += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("image Search: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// GetDistinctYears returns distinct non-null years ordered DESC.
func (r *ImageRepo) GetDistinctYears(ctx context.Context) ([]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT year FROM media_items WHERE year IS NOT NULL ORDER BY year DESC`)
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
	rows, err := r.pool.Query(ctx,
		`SELECT tags FROM media_items WHERE tags IS NOT NULL AND tags != ''`)
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

// GetLocations returns media_items with GPS data (has_gps=true or lat/lng non-null).
func (r *ImageRepo) GetLocations(ctx context.Context) ([]*model.MediaItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+mediaItemColumns+` FROM media_items
		 WHERE has_gps = TRUE
		    OR (latitude IS NOT NULL AND longitude IS NOT NULL)`)
	if err != nil {
		return nil, fmt.Errorf("GetLocations: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// GetFacebookPlaces returns all locations with source='facebook'.
func (r *ImageRepo) GetFacebookPlaces(ctx context.Context) ([]model.FacebookPlaceItem, error) {
	rows, err := r.pool.Query(ctx, `
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

// UpdateTags updates the tags column for a media item (merge: append to existing).
func (r *ImageRepo) UpdateTags(ctx context.Context, id int64, newTags string) (bool, error) {
	var existing *string
	err := r.pool.QueryRow(ctx, `SELECT tags FROM media_items WHERE id = $1`, id).Scan(&existing)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("UpdateTags: %w", err)
	}
	merged := newTags
	if existing != nil && strings.TrimSpace(*existing) != "" {
		merged = strings.TrimSpace(*existing) + ", " + strings.TrimSpace(newTags)
	}
	res, err := r.pool.Exec(ctx, `UPDATE media_items SET tags = $2, updated_at = NOW() WHERE id = $1`, id, merged)
	if err != nil {
		return false, fmt.Errorf("UpdateTags: %w", err)
	}
	return res.RowsAffected() > 0, nil
}

// UpdateMetadata updates description, tags, and/or rating for a media item.
func (r *ImageRepo) UpdateMetadata(ctx context.Context, id int64, description, tags *string, rating *int) (bool, error) {
	item, err := r.GetMediaItemByID(ctx, id)
	if err != nil || item == nil {
		return false, nil
	}
	var setParts []string
	var args []any
	n := 1
	args = append(args, id)
	n++
	if description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", n))
		args = append(args, *description)
		n++
	}
	if tags != nil {
		setParts = append(setParts, fmt.Sprintf("tags = $%d", n))
		args = append(args, *tags)
		n++
	}
	if rating != nil {
		setParts = append(setParts, fmt.Sprintf("rating = $%d", n))
		args = append(args, *rating)
		n++
	}
	if len(setParts) == 0 {
		return true, nil
	}
	setParts = append(setParts, "updated_at = NOW()")
	sql := fmt.Sprintf(`UPDATE media_items SET %s WHERE id = $1`, strings.Join(setParts, ", "))
	res, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("UpdateMetadata: %w", err)
	}
	return res.RowsAffected() > 0, nil
}

// DeleteByMetadataID deletes a media_items row and its media_blobs row.
func (r *ImageRepo) DeleteByMetadataID(ctx context.Context, id int64) (bool, error) {
	var blobID int64
	err := r.pool.QueryRow(ctx, `SELECT media_blob_id FROM media_items WHERE id = $1`, id).Scan(&blobID)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("DeleteByMetadataID: %w", err)
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM media_items WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("DeleteByMetadataID: %w", err)
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM media_blobs WHERE id = $1`, blobID)
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

	var cond string
	var args []any
	n := 1
	if all {
		cond = "media_type LIKE 'image/%'"
	} else {
		parts := []string{"media_type LIKE 'image/%'"}
		if startID != nil {
			parts = append(parts, fmt.Sprintf("id >= $%d", n))
			args = append(args, *startID)
			n++
		}
		if endID != nil {
			parts = append(parts, fmt.Sprintf("id <= $%d", n))
			args = append(args, *endID)
		}
		cond = strings.Join(parts, " AND ")
	}
	rows, err := r.pool.Query(ctx, `SELECT id, media_blob_id FROM media_items WHERE `+cond, args...)
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
	_, err = r.pool.Exec(ctx, `DELETE FROM media_items WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, fmt.Errorf("DeleteByIDRange: %w", err)
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM media_blobs WHERE id = ANY($1)`, blobIDs)
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
	rows, err := r.pool.Query(ctx,
		`SELECT id, media_blob_id, COALESCE(source_reference,'') FROM media_items
		 WHERE is_referenced = TRUE AND source_reference IS NOT NULL AND source_reference != ''`)
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

// UpdateBlobImageDataAndClearReferenced updates media_blobs.image_data and sets media_items.is_referenced=false.
func (r *ImageRepo) UpdateBlobImageDataAndClearReferenced(ctx context.Context, itemID, blobID int64, data []byte) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_blobs SET image_data = $2 WHERE id = $1`, blobID, data)
	if err != nil {
		return fmt.Errorf("UpdateBlobImageData: %w", err)
	}
	_, err = r.pool.Exec(ctx, `UPDATE media_items SET is_referenced = FALSE, updated_at = NOW() WHERE id = $1`, itemID)
	if err != nil {
		return fmt.Errorf("SetItemNotReferenced: %w", err)
	}
	return nil
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
	rows, err := r.pool.Query(ctx,
		`SELECT id, media_blob_id, title, media_type, source_reference FROM media_items ORDER BY id`)
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
	err := r.pool.QueryRow(ctx, `SELECT image_data FROM media_blobs WHERE id = $1`, blobID).Scan(&data)
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
	rows, err := r.pool.Query(ctx, `
		SELECT fa.id, fa.name, fa.description, fa.cover_photo_uri,
		       COUNT(DISTINCT am.id) AS image_count
		FROM facebook_albums fa
		LEFT JOIN album_media am ON fa.id = am.album_id
		GROUP BY fa.id, fa.name, fa.description, fa.cover_photo_uri
		ORDER BY fa.name`)
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
	rows, err := r.pool.Query(ctx,
		`SELECT `+mediaItemColumnsQualified+`
		 FROM media_items mi
		 JOIN album_media am ON mi.id = am.media_item_id
		 WHERE am.album_id = $1
		 ORDER BY mi.created_at ASC`, albumID)
	if err != nil {
		return nil, fmt.Errorf("GetAlbumImages: %w", err)
	}
	defer rows.Close()
	return scanMediaItems(rows)
}

// GetAlbumImageByID returns a media_items row that is linked to any album.
func (r *ImageRepo) GetAlbumImageByID(ctx context.Context, imageID int64) (*model.MediaItem, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+mediaItemColumnsQualified+`
		 FROM media_items mi
		 JOIN album_media am ON mi.id = am.media_item_id
		 WHERE mi.id = $1
		 LIMIT 1`, imageID)
	return scanMediaItem(row)
}

// GetFacebookPostsParams holds optional filters for GetFacebookPosts.
type GetFacebookPostsParams struct {
	Search   string  // ILIKE on post_text and title
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

	baseQuery := `
		SELECT fp.id, fp.timestamp, fp.title, fp.post_text, fp.external_url, fp.post_type,
		       COUNT(DISTINCT pm.id)::int AS media_count
		FROM facebook_posts fp
		LEFT JOIN post_media pm ON fp.id = pm.post_id
	`

	var args []any
	var conds []string
	argNum := 1

	if p.Search != "" {
		conds = append(conds, fmt.Sprintf("(fp.post_text ILIKE $%d OR fp.title ILIKE $%d)", argNum, argNum))
		args = append(args, "%"+p.Search+"%")
		argNum++
	}
	if len(p.PostIDs) > 0 {
		conds = append(conds, fmt.Sprintf("fp.id = ANY($%d)", argNum))
		args = append(args, p.PostIDs)
		argNum++
	}

	if len(conds) > 0 {
		baseQuery += " WHERE " + strings.Join(conds, " AND ")
	}
	baseQuery += " GROUP BY fp.id, fp.timestamp, fp.title, fp.post_text, fp.external_url, fp.post_type"
	baseQuery += " ORDER BY fp.timestamp DESC NULLS LAST"

	// Count total
	countQuery := `SELECT COUNT(*) FROM (` + baseQuery + `) AS sub`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("GetFacebookPosts count: %w", err)
	}

	// Fetch page
	offset := (p.Page - 1) * p.PageSize
	args = append(args, p.PageSize, offset)
	pageQuery := baseQuery + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argNum, argNum+1)

	rows, err := r.pool.Query(ctx, pageQuery, args...)
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
	row := r.pool.QueryRow(ctx,
		`SELECT `+mediaItemColumnsQualified+`
		 FROM media_items mi
		 JOIN post_media pm ON mi.id = pm.media_item_id
		 WHERE mi.id = $1
		 LIMIT 1`, mediaID)
	return scanMediaItem(row)
}

// GetPostMedia returns media_items linked to a Facebook post via post_media, ordered by created_at ASC.
func (r *ImageRepo) GetPostMedia(ctx context.Context, postID int64) ([]*model.MediaItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+mediaItemColumnsQualified+`
		 FROM media_items mi
		 JOIN post_media pm ON mi.id = pm.media_item_id
		 WHERE pm.post_id = $1
		 ORDER BY mi.created_at ASC`, postID)
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
		&m.Year, &m.Month, &m.Latitude, &m.Longitude, &m.Altitude,
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
			&m.Year, &m.Month, &m.Latitude, &m.Longitude, &m.Altitude,
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

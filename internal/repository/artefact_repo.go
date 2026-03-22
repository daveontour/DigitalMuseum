package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArtefactRepo accesses artefacts, artefact_media, media_items, and media_blobs.
type ArtefactRepo struct {
	pool *pgxpool.Pool
}

// NewArtefactRepo creates an ArtefactRepo.
func NewArtefactRepo(pool *pgxpool.Pool) *ArtefactRepo {
	return &ArtefactRepo{pool: pool}
}

// ── Artefact basic CRUD ────────────────────────────────────────────────────────

// ListSummaries returns all artefacts with optional search/tag filtering.
// The primary_thumbnail_url is derived from the first linked media blob.
func (r *ArtefactRepo) ListSummaries(ctx context.Context, search, tags string) ([]*model.ArtefactSummary, error) {
	q := `
		SELECT a.id, a.name, a.description, a.tags, a.created_at, a.updated_at,
		       (SELECT mb.id
		        FROM artefact_media am
		        JOIN media_items mi ON mi.id = am.media_item_id
		        JOIN media_blobs mb ON mb.id = mi.media_blob_id
		        WHERE am.artefact_id = a.id
		          AND (
		            (mb.thumbnail_data IS NOT NULL AND octet_length(mb.thumbnail_data) > 0)
		            OR (mi.media_type IS NOT NULL AND mi.media_type ILIKE 'image/%')
		          )
		        ORDER BY am.sort_order
		        LIMIT 1) AS primary_blob_id
		FROM artefacts a`
	var args []any
	var conds []string
	if search != "" {
		args = append(args, "%"+search+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf(
			`(a.name ILIKE $%d OR a.description ILIKE $%d OR a.tags ILIKE $%d OR a.story ILIKE $%d)`,
			idx, idx, idx, idx,
		))
	}
	if tags != "" {
		args = append(args, "%"+tags+"%")
		idx := len(args)
		conds = append(conds, fmt.Sprintf("a.tags ILIKE $%d", idx))
	}
	if len(conds) > 0 {
		q += " WHERE " + joinAnd(conds)
	}
	q += " ORDER BY a.updated_at DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListSummaries: %w", err)
	}
	defer rows.Close()

	var out []*model.ArtefactSummary
	for rows.Next() {
		var a model.ArtefactSummary
		var blobID *int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.Tags,
			&a.CreatedAt, &a.UpdatedAt, &blobID); err != nil {
			return nil, err
		}
		if blobID != nil {
			url := fmt.Sprintf("/images/%d?preview=true&type=blob", *blobID)
			a.PrimaryThumbnailURL = &url
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// GetByID returns a single artefact row (no media).
func (r *ArtefactRepo) GetByID(ctx context.Context, id int64) (*model.Artefact, error) {
	var a model.Artefact
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, tags, story, created_at, updated_at
		 FROM artefacts WHERE id = $1`, id,
	).Scan(&a.ID, &a.Name, &a.Description, &a.Tags, &a.Story, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetArtefactByID %d: %w", id, err)
	}
	return &a, nil
}

// GetMediaItems returns all linked media items for an artefact, ordered by sort_order.
func (r *ArtefactRepo) GetMediaItems(ctx context.Context, artefactID int64) ([]*model.ArtefactMediaItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT am.id, am.media_item_id, mi.media_blob_id, am.sort_order, mi.media_type, mi.title,
		        COALESCE(octet_length(mb.thumbnail_data), 0)::bigint AS thumb_len
		 FROM artefact_media am
		 JOIN media_items mi ON mi.id = am.media_item_id
		 JOIN media_blobs mb ON mb.id = mi.media_blob_id
		 WHERE am.artefact_id = $1
		 ORDER BY am.sort_order`, artefactID)
	if err != nil {
		return nil, fmt.Errorf("GetMediaItems %d: %w", artefactID, err)
	}
	defer rows.Close()

	var out []*model.ArtefactMediaItem
	for rows.Next() {
		var item model.ArtefactMediaItem
		var thumbLen int64
		if err := rows.Scan(&item.ID, &item.MediaItemID, &item.MediaBlobID,
			&item.SortOrder, &item.MediaType, &item.Title, &thumbLen); err != nil {
			return nil, err
		}
		if item.MediaBlobID != nil {
			mt := ""
			if item.MediaType != nil {
				mt = strings.ToLower(*item.MediaType)
				if i := strings.Index(mt, ";"); i >= 0 {
					mt = strings.TrimSpace(mt[:i])
				}
			}
			if thumbLen > 0 {
				item.ThumbnailURL = fmt.Sprintf("/images/%d?preview=true&type=blob", *item.MediaBlobID)
			} else if strings.HasPrefix(mt, "image/") {
				item.ThumbnailURL = fmt.Sprintf("/images/%d?type=blob", *item.MediaBlobID)
			}
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

// Create inserts a new artefact and returns it.
func (r *ArtefactRepo) Create(ctx context.Context, name string, description, tags, story *string) (*model.Artefact, error) {
	var a model.Artefact
	err := r.pool.QueryRow(ctx,
		`INSERT INTO artefacts (name, description, tags, story)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, description, tags, story, created_at, updated_at`,
		name, description, tags, story,
	).Scan(&a.ID, &a.Name, &a.Description, &a.Tags, &a.Story, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateArtefact: %w", err)
	}
	return &a, nil
}

// Update modifies artefact fields (nil values are left unchanged).
func (r *ArtefactRepo) Update(ctx context.Context, id int64, name *string, description, tags, story *string) (*model.Artefact, error) {
	var a model.Artefact
	err := r.pool.QueryRow(ctx,
		`UPDATE artefacts
		 SET name        = COALESCE($1, name),
		     description = COALESCE($2, description),
		     tags        = COALESCE($3, tags),
		     story       = COALESCE($4, story),
		     updated_at  = NOW()
		 WHERE id = $5
		 RETURNING id, name, description, tags, story, created_at, updated_at`,
		name, description, tags, story, id,
	).Scan(&a.ID, &a.Name, &a.Description, &a.Tags, &a.Story, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("UpdateArtefact %d: %w", id, err)
	}
	return &a, nil
}

// TouchUpdatedAt sets updated_at = NOW() on an artefact.
func (r *ArtefactRepo) TouchUpdatedAt(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE artefacts SET updated_at = NOW() WHERE id = $1`, id)
	return err
}

// Delete removes an artefact. Caller is responsible for cleaning up owned media first.
func (r *ArtefactRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM artefacts WHERE id = $1`, id)
	return err
}

// ── Media linking ─────────────────────────────────────────────────────────────

// MediaLinkExists reports whether a junction row already exists.
func (r *ArtefactRepo) MediaLinkExists(ctx context.Context, artefactID, mediaItemID int64) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM artefact_media WHERE artefact_id=$1 AND media_item_id=$2`,
		artefactID, mediaItemID,
	).Scan(&n)
	return n > 0, err
}

// MediaLinkCount returns the number of media items linked to an artefact.
func (r *ArtefactRepo) MediaLinkCount(ctx context.Context, artefactID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM artefact_media WHERE artefact_id=$1`, artefactID,
	).Scan(&n)
	return n, err
}

// LinkMedia creates a junction row.
func (r *ArtefactRepo) LinkMedia(ctx context.Context, artefactID, mediaItemID int64, sortOrder int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO artefact_media (artefact_id, media_item_id, sort_order)
		 VALUES ($1, $2, $3)`,
		artefactID, mediaItemID, sortOrder)
	return err
}

// UnlinkMedia removes a junction row.
func (r *ArtefactRepo) UnlinkMedia(ctx context.Context, artefactID, mediaItemID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM artefact_media WHERE artefact_id=$1 AND media_item_id=$2`,
		artefactID, mediaItemID)
	return err
}

// OtherArtefactLinkCount returns how many OTHER artefacts also link this media item.
func (r *ArtefactRepo) OtherArtefactLinkCount(ctx context.Context, mediaItemID, excludeArtefactID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM artefact_media WHERE media_item_id=$1 AND artefact_id!=$2`,
		mediaItemID, excludeArtefactID,
	).Scan(&n)
	return n, err
}

// ── Media blob/metadata creation (for upload) ──────────────────────────────────

// InsertMediaBlob inserts a new media_blobs row and returns the new id.
func (r *ArtefactRepo) InsertMediaBlob(ctx context.Context, imageData, thumbnailData []byte) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, $2) RETURNING id`,
		imageData, thumbnailData,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("InsertMediaBlob: %w", err)
	}
	return id, nil
}

// InsertMediaItem inserts a new media_items row and returns the new id.
func (r *ArtefactRepo) InsertMediaItem(ctx context.Context, blobID int64, title, mediaType, source, sourceRef string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO media_items (media_blob_id, title, media_type, source, source_reference, processed)
		 VALUES ($1, $2, $3, $4, $5, TRUE) RETURNING id`,
		blobID, title, mediaType, source, sourceRef,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("InsertMediaItem: %w", err)
	}
	return id, nil
}

// GetMediaItemSource returns the source field of a media_items row.
func (r *ArtefactRepo) GetMediaItemSource(ctx context.Context, mediaItemID int64) (string, error) {
	var source string
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(source, '') FROM media_items WHERE id=$1`, mediaItemID,
	).Scan(&source)
	if err != nil {
		if isNoRows(err) {
			return "", nil
		}
		return "", err
	}
	return source, nil
}

// GetMediaItemBlobID returns the media_blob_id for a given media_items row.
func (r *ArtefactRepo) GetMediaItemBlobID(ctx context.Context, mediaItemID int64) (*int64, error) {
	var blobID *int64
	err := r.pool.QueryRow(ctx,
		`SELECT media_blob_id FROM media_items WHERE id=$1`, mediaItemID,
	).Scan(&blobID)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return blobID, nil
}

// DeleteMediaItem deletes a media_items row.
func (r *ArtefactRepo) DeleteMediaItem(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM media_items WHERE id=$1`, id)
	return err
}

// DeleteMediaBlob deletes a media_blobs row.
func (r *ArtefactRepo) DeleteMediaBlob(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM media_blobs WHERE id=$1`, id)
	return err
}

// GetPrimaryBlob returns (image_data, thumbnail_data) for the first linked media of an artefact.
func (r *ArtefactRepo) GetPrimaryBlob(ctx context.Context, artefactID int64) ([]byte, error) {
	var data, thumb []byte
	err := r.pool.QueryRow(ctx,
		`SELECT mb.image_data, mb.thumbnail_data
		 FROM artefact_media am
		 JOIN media_items mi ON mi.id = am.media_item_id
		 JOIN media_blobs mb ON mb.id = mi.media_blob_id
		 WHERE am.artefact_id = $1
		 ORDER BY am.sort_order
		 LIMIT 1`, artefactID,
	).Scan(&data, &thumb)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetPrimaryBlob: %w", err)
	}
	if len(thumb) > 0 {
		return thumb, nil
	}
	return data, nil
}

// ── Export / Import helpers ────────────────────────────────────────────────────

// ArtefactExportRow is used by the export endpoint.
type ArtefactExportRow struct {
	Artefact  model.Artefact
	MediaRefs []ArtefactMediaRef
}

// ArtefactMediaRef holds the re-linkable reference for an exported media item.
type ArtefactMediaRef struct {
	SortOrder       int
	MediaType       *string
	Title           *string
	Source          *string
	SourceReference *string
}

// ExportAll returns all artefacts with media refs.
func (r *ArtefactRepo) ExportAll(ctx context.Context) ([]*ArtefactExportRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, tags, story, created_at, updated_at FROM artefacts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ArtefactExportRow
	for rows.Next() {
		var row ArtefactExportRow
		if err := rows.Scan(&row.Artefact.ID, &row.Artefact.Name,
			&row.Artefact.Description, &row.Artefact.Tags, &row.Artefact.Story,
			&row.Artefact.CreatedAt, &row.Artefact.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load media refs for each artefact
	for _, row := range out {
		mrRows, err := r.pool.Query(ctx,
			`SELECT am.sort_order, mi.media_type, mi.title, mi.source, mi.source_reference
			 FROM artefact_media am
			 JOIN media_items mi ON mi.id = am.media_item_id
			 WHERE am.artefact_id = $1
			 ORDER BY am.sort_order`, row.Artefact.ID)
		if err != nil {
			return nil, err
		}
		for mrRows.Next() {
			var ref ArtefactMediaRef
			if err := mrRows.Scan(&ref.SortOrder, &ref.MediaType, &ref.Title, &ref.Source, &ref.SourceReference); err != nil {
				mrRows.Close()
				return nil, err
			}
			row.MediaRefs = append(row.MediaRefs, ref)
		}
		mrRows.Close()
		if err := mrRows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// FindMediaBySrcRef looks up a media_items.id by source + source_reference.
func (r *ArtefactRepo) FindMediaBySrcRef(ctx context.Context, source, sourceRef string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM media_items WHERE source=$1 AND source_reference=$2 LIMIT 1`,
		source, sourceRef,
	).Scan(&id)
	if err != nil {
		if isNoRows(err) {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

// GetOrphanArtefactMediaIDs returns media_item IDs that are linked only to the given artefact
// and have source='artefact'.
func (r *ArtefactRepo) GetOrphanArtefactMediaIDs(ctx context.Context, artefactID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT am.media_item_id
		 FROM artefact_media am
		 JOIN media_items mi ON mi.id = am.media_item_id
		 WHERE am.artefact_id = $1
		   AND mi.source = 'artefact'
		   AND NOT EXISTS (
		       SELECT 1 FROM artefact_media am2
		       WHERE am2.media_item_id = am.media_item_id
		         AND am2.artefact_id != $1
		   )`, artefactID)
	if err != nil {
		return nil, err
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

// ── helpers ────────────────────────────────────────────────────────────────────

func joinAnd(parts []string) string {
	result := parts[0]
	for _, p := range parts[1:] {
		result += " AND " + p
	}
	return result
}

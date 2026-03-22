package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AttachmentRepo accesses IMAP and Gmail email attachment rows in media_items.
type AttachmentRepo struct {
	pool *pgxpool.Pool
}

// emailAttachmentSourcesSQL matches media_items.source for stored email attachments (IMAP + Gmail).
const emailAttachmentSourcesSQL = `mm.source IN ('email_attachment', 'gmail_attachment')`

// NewAttachmentRepo creates an AttachmentRepo.
func NewAttachmentRepo(pool *pgxpool.Pool) *AttachmentRepo {
	return &AttachmentRepo{pool: pool}
}

// attachmentInfoCols joins media_items to emails (source_reference holds string email id).
const attachmentInfoCols = `
	mm.id AS attachment_id,
	COALESCE(mm.title, 'attachment') AS filename,
	COALESCE(mm.media_type, 'application/octet-stream') AS content_type,
	e.id AS email_id,
	e.subject AS email_subject,
	e.from_address AS email_from,
	e.date AS email_date,
	e.folder AS email_folder`

func scanAttachmentInfo(row interface{ Scan(...any) error }) (*model.AttachmentInfo, error) {
	var a model.AttachmentInfo
	err := row.Scan(
		&a.AttachmentID, &a.Filename, &a.ContentType,
		&a.EmailID, &a.EmailSubject, &a.EmailFrom, &a.EmailDate, &a.EmailFolder,
	)
	return &a, err
}

// GetRandom returns one random email attachment media item with email metadata.
func (r *AttachmentRepo) GetRandom(ctx context.Context) (*model.AttachmentInfo, error) {
	a, err := scanAttachmentInfo(r.pool.QueryRow(ctx, `
		SELECT `+attachmentInfoCols+`
		FROM media_items mm
		JOIN emails e ON e.id = mm.source_reference::bigint
		WHERE `+emailAttachmentSourcesSQL+`
		ORDER BY RANDOM()
		LIMIT 1`))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetRandomAttachment: %w", err)
	}
	return a, nil
}

// GetByIDOrder returns an attachment ordered by media_items.id with an offset.
func (r *AttachmentRepo) GetByIDOrder(ctx context.Context, offset int) (*model.AttachmentInfo, error) {
	a, err := scanAttachmentInfo(r.pool.QueryRow(ctx, `
		SELECT `+attachmentInfoCols+`
		FROM media_items mm
		JOIN emails e ON e.id = mm.source_reference::bigint
		WHERE `+emailAttachmentSourcesSQL+`
		ORDER BY mm.id ASC
		OFFSET $1
		LIMIT 1`, offset))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetAttachmentByIDOrder: %w", err)
	}
	return a, nil
}

// GetBySize returns one attachment ordered by blob size, with an offset.
func (r *AttachmentRepo) GetBySize(ctx context.Context, orderDesc bool, offset int) (*model.AttachmentInfo, error) {
	dir := "ASC NULLS LAST"
	if orderDesc {
		dir = "DESC NULLS LAST"
	}
	q := fmt.Sprintf(`
		SELECT `+attachmentInfoCols+`, octet_length(mb.image_data) AS sz
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		JOIN emails e ON e.id = mm.source_reference::bigint
		WHERE `+emailAttachmentSourcesSQL+`
		ORDER BY octet_length(mb.image_data) %s
		OFFSET $1
		LIMIT 1`, dir)
	var a model.AttachmentInfo
	var sz *int64
	err := r.pool.QueryRow(ctx, q, offset).Scan(
		&a.AttachmentID, &a.Filename, &a.ContentType,
		&a.EmailID, &a.EmailSubject, &a.EmailFrom, &a.EmailDate, &a.EmailFolder,
		&sz,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetAttachmentBySize: %w", err)
	}
	a.Size = sz
	return &a, nil
}

// Count returns the total number of IMAP/Gmail email attachment media items.
func (r *AttachmentRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM media_items WHERE source IN ('email_attachment', 'gmail_attachment')`).Scan(&n)
	return n, err
}

// attachmentInfoColsGetInfo uses COALESCE(e.id, 0) so LEFT JOIN emails never yields NULL email_id into int64.
const attachmentInfoColsGetInfo = `
	mm.id AS attachment_id,
	COALESCE(mm.title, 'attachment') AS filename,
	COALESCE(mm.media_type, 'application/octet-stream') AS content_type,
	COALESCE(e.id, 0) AS email_id,
	e.subject AS email_subject,
	e.from_address AS email_from,
	e.date AS email_date,
	e.folder AS email_folder`

// GetInfo returns metadata for a single attachment.
func (r *AttachmentRepo) GetInfo(ctx context.Context, id int64) (*model.AttachmentInfo, error) {
	a, err := scanAttachmentInfo(r.pool.QueryRow(ctx, `
		SELECT `+attachmentInfoColsGetInfo+`
		FROM media_items mm
		LEFT JOIN emails e ON e.id = mm.source_reference::bigint
		WHERE `+emailAttachmentSourcesSQL+` AND mm.id = $1`, id))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetAttachmentInfo: %w", err)
	}
	return a, nil
}

// GetData returns the raw blob data and thumbnail for an attachment.
func (r *AttachmentRepo) GetData(ctx context.Context, id int64) (data, thumbnail []byte, mediaType string, filename string, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT mb.image_data, mb.thumbnail_data, COALESCE(mm.media_type,'application/octet-stream'), COALESCE(mm.title,'attachment')
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		WHERE `+emailAttachmentSourcesSQL+` AND mm.id = $1`, id).
		Scan(&data, &thumbnail, &mediaType, &filename)
	if err != nil {
		if isNoRows(err) {
			err = nil
			return nil, nil, "", "", nil
		}
		err = fmt.Errorf("GetAttachmentData: %w", err)
	}
	return
}

// Delete removes a media_items row and its blob if no other media_items reference it.
func (r *AttachmentRepo) Delete(ctx context.Context, id int64) (bool, error) {
	var blobID *int64
	err := r.pool.QueryRow(ctx,
		`SELECT media_blob_id FROM media_items WHERE id=$1 AND source IN ('email_attachment', 'gmail_attachment')`, id).
		Scan(&blobID)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, err
	}

	if _, err := r.pool.Exec(ctx, `DELETE FROM media_items WHERE id=$1`, id); err != nil {
		return false, err
	}

	if blobID != nil {
		var refs int
		if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM media_items WHERE media_blob_id=$1`, *blobID).Scan(&refs); err == nil && refs == 0 {
			_, _ = r.pool.Exec(ctx, `DELETE FROM media_blobs WHERE id=$1`, *blobID)
		}
	}
	return true, nil
}

// ListImages returns a paginated, sorted list of image (or all) attachments.
func (r *AttachmentRepo) ListImages(ctx context.Context, page, pageSize int, order, direction string, allTypes bool) ([]*model.AttachmentInfo, int64, error) {
	var typeFilter string
	if !allTypes {
		typeFilter = " AND mm.media_type LIKE 'image/%'"
	}

	var orderExpr string
	switch order {
	case "size":
		if direction == "desc" {
			orderExpr = "octet_length(mb.image_data) DESC NULLS LAST"
		} else {
			orderExpr = "octet_length(mb.image_data) ASC NULLS LAST"
		}
	case "date":
		if direction == "desc" {
			orderExpr = "e.date DESC NULLS LAST"
		} else {
			orderExpr = "e.date ASC NULLS LAST"
		}
	default: // "id"
		if direction == "desc" {
			orderExpr = "mm.id DESC"
		} else {
			orderExpr = "mm.id ASC"
		}
	}

	offset := (page - 1) * pageSize

	var total int64
	if err := r.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		JOIN emails e ON e.id = mm.source_reference::bigint
		WHERE `+emailAttachmentSourcesSQL+`%s`, typeFilter)).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListImagesCount: %w", err)
	}

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT mm.id, COALESCE(mm.title,'attachment'), COALESCE(mm.media_type,'application/octet-stream'),
		       e.id, e.subject, e.from_address, e.date, e.folder,
		       octet_length(mb.image_data) AS sz
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		JOIN emails e ON e.id = mm.source_reference::bigint
		WHERE `+emailAttachmentSourcesSQL+`%s
		ORDER BY %s
		LIMIT $1 OFFSET $2`, typeFilter, orderExpr), pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListImages: %w", err)
	}
	defer rows.Close()

	var out []*model.AttachmentInfo
	for rows.Next() {
		var a model.AttachmentInfo
		var sz *int64
		if err := rows.Scan(
			&a.AttachmentID, &a.Filename, &a.ContentType,
			&a.EmailID, &a.EmailSubject, &a.EmailFrom, &a.EmailDate, &a.EmailFolder,
			&sz,
		); err != nil {
			return nil, 0, err
		}
		a.Size = sz
		out = append(out, &a)
	}
	return out, total, rows.Err()
}

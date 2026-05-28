package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// AttachmentRepo accesses IMAP and Gmail email attachment rows in media_items.
type AttachmentRepo struct {
	pool *sql.DB
}

// emailAttachmentSourcesSQL matches media_items.source for stored email attachments (IMAP + Gmail).
const emailAttachmentSourcesSQL = `mm.source IN ('email_attachment', 'gmail_attachment')`

// emailIDJoinExpr compares emails.id to string-stored source_reference (PostgreSQL ::bigint is not portable).
const emailIDJoinExpr = `CAST(mm.source_reference AS INTEGER)`

// NewAttachmentRepo creates an AttachmentRepo.
func NewAttachmentRepo(pool *sql.DB) *AttachmentRepo {
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
	uid := uidFromCtx(ctx)
	q := `
		SELECT ` + attachmentInfoCols + `
		FROM media_items mm
		JOIN emails e ON e.id = ` + emailIDJoinExpr + `
		WHERE ` + emailAttachmentSourcesSQL
	args := []any{}
	// Use qualified alias — media_items mm and emails e both have user_id
	q, args = addUIDFilterQualified(q, args, uid, "mm")
	q += `
		ORDER BY RANDOM()
		LIMIT 1`
	a, err := scanAttachmentInfo(r.pool.QueryRowContext(ctx, q, args...))
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
	uid := uidFromCtx(ctx)
	q := `
		SELECT ` + attachmentInfoCols + `
		FROM media_items mm
		JOIN emails e ON e.id = ` + emailIDJoinExpr + `
		WHERE ` + emailAttachmentSourcesSQL
	args := []any{}
	// Use qualified alias — media_items mm and emails e both have user_id
	q, args = addUIDFilterQualified(q, args, uid, "mm")
	args = append(args, offset)
	q += fmt.Sprintf(`
		ORDER BY mm.id ASC
		OFFSET ?%d
		LIMIT 1`, len(args))
	a, err := scanAttachmentInfo(r.pool.QueryRowContext(ctx, q, args...))
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
	uid := uidFromCtx(ctx)
	orderExpr := sqlutil.OrderByAscNullsLast("LENGTH(mb.image_data)")
	if orderDesc {
		orderExpr = sqlutil.OrderByDescNullsLast("LENGTH(mb.image_data)")
	}
	q := `
		SELECT ` + attachmentInfoCols + `, LENGTH(mb.image_data) AS sz
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		JOIN emails e ON e.id = ` + emailIDJoinExpr + `
		WHERE ` + emailAttachmentSourcesSQL
	args := []any{}
	// Use qualified alias — media_items mm, media_blobs mb, and emails e all have user_id
	q, args = addUIDFilterQualified(q, args, uid, "mm")
	args = append(args, offset)
	q += fmt.Sprintf(`
		ORDER BY %s
		OFFSET ?%d
		LIMIT 1`, orderExpr, len(args))
	var a model.AttachmentInfo
	var sz *int64
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(
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
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM media_items WHERE source IN ('email_attachment', 'gmail_attachment')`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	var n int64
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n)
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
	uid := uidFromCtx(ctx)
	q := `
		SELECT ` + attachmentInfoColsGetInfo + `
		FROM media_items mm
		LEFT JOIN emails e ON e.id = ` + emailIDJoinExpr + `
		WHERE ` + emailAttachmentSourcesSQL + ` AND mm.id = ?1`
	args := []any{id}
	// Use qualified alias — media_items mm and emails e both have user_id
	q, args = addUIDFilterQualified(q, args, uid, "mm")
	a, err := scanAttachmentInfo(r.pool.QueryRowContext(ctx, q, args...))
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
	uid := uidFromCtx(ctx)
	q := `
		SELECT mb.image_data, mb.thumbnail_data, COALESCE(mm.media_type,'application/octet-stream'), COALESCE(mm.title,'attachment')
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		WHERE ` + emailAttachmentSourcesSQL + ` AND mm.id = ?1`
	args := []any{id}
	// Use qualified alias — media_items mm and media_blobs mb both have user_id
	q, args = addUIDFilterQualified(q, args, uid, "mm")
	err = r.pool.QueryRowContext(ctx, q, args...).
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
	err := r.pool.QueryRowContext(ctx,
		`SELECT media_blob_id FROM media_items WHERE id=?1 AND source IN ('email_attachment', 'gmail_attachment')`, id).
		Scan(&blobID)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, err
	}

	if _, err := r.pool.ExecContext(ctx, `DELETE FROM media_items WHERE id=?1`, id); err != nil {
		return false, err
	}

	if blobID != nil {
		var refs int
		if err := r.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_items WHERE media_blob_id=?1`, *blobID).Scan(&refs); err == nil && refs == 0 {
			_, _ = r.pool.ExecContext(ctx, `DELETE FROM media_blobs WHERE id=?1`, *blobID)
		}
	}
	return true, nil
}

// ListImages returns a paginated, sorted list of image (or all) attachments.
func (r *AttachmentRepo) ListImages(ctx context.Context, page, pageSize int, order, direction string, allTypes bool) ([]*model.AttachmentInfo, int64, error) {
	uid := uidFromCtx(ctx)
	var typeFilter string
	if !allTypes {
		typeFilter = " AND mm.media_type LIKE 'image/%'"
	}

	var orderExpr string
	switch order {
	case "size":
		if direction == "desc" {
			orderExpr = sqlutil.OrderByDescNullsLast("LENGTH(mb.image_data)")
		} else {
			orderExpr = sqlutil.OrderByAscNullsLast("LENGTH(mb.image_data)")
		}
	case "date":
		if direction == "desc" {
			orderExpr = sqlutil.OrderByDescNullsLast("e.date")
		} else {
			orderExpr = sqlutil.OrderByAscNullsLast("e.date")
		}
	default: // "id"
		if direction == "desc" {
			orderExpr = "mm.id DESC"
		} else {
			orderExpr = "mm.id ASC"
		}
	}

	offset := (page - 1) * pageSize

	// Build count query
	countBase := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		JOIN emails e ON e.id = `+emailIDJoinExpr+`
		WHERE `+emailAttachmentSourcesSQL+`%s`, typeFilter)
	countArgs := []any{}
	// Use qualified alias — media_items mm, media_blobs mb, and emails e all have user_id
	countBase, countArgs = addUIDFilterQualified(countBase, countArgs, uid, "mm")

	var total int64
	if err := r.pool.QueryRowContext(ctx, countBase, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListImagesCount: %w", err)
	}

	// Build list query
	listBase := fmt.Sprintf(`
		SELECT mm.id, COALESCE(mm.title,'attachment'), COALESCE(mm.media_type,'application/octet-stream'),
		       e.id, e.subject, e.from_address, e.date, e.folder,
		       LENGTH(mb.image_data) AS sz
		FROM media_items mm
		JOIN media_blobs mb ON mb.id = mm.media_blob_id
		JOIN emails e ON e.id = `+emailIDJoinExpr+`
		WHERE `+emailAttachmentSourcesSQL+`%s`, typeFilter)
	listArgs := []any{}
	// Use qualified alias — media_items mm, media_blobs mb, and emails e all have user_id
	listBase, listArgs = addUIDFilterQualified(listBase, listArgs, uid, "mm")
	listArgs = append(listArgs, pageSize, offset)
	listBase += fmt.Sprintf(`
		ORDER BY %s
		LIMIT ?%d OFFSET ?%d`, orderExpr, len(listArgs)-1, len(listArgs))

	rows, err := r.pool.QueryContext(ctx, listBase, listArgs...)
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

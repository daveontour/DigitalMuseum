package sqlutil

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const mediaItemsPurgeTempTable = "temp_media_items_purge_blob_ids"
const emailAttachmentBlobPurgeTempTable = "temp_email_attachment_blob_ids"

// DeleteMediaItemsByUserAndSourceTx deletes all media_items for the given user and source,
// then deletes the corresponding media_blobs rows. It stages blob IDs in a TEMP table so
// this works on SQLite before 3.35, which does not allow DELETE inside a WITH clause.
// Returns the number of media_blobs rows removed (same as the outer DELETE in the old CTE form).
func DeleteMediaItemsByUserAndSourceTx(ctx context.Context, tx *sql.Tx, userID int64, source string) (blobsDeleted int64, err error) {
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS `+mediaItemsPurgeTempTable+` (blob_id INTEGER)`); err != nil {
		return 0, fmt.Errorf("create purge temp table: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM `+mediaItemsPurgeTempTable); err != nil {
		return 0, fmt.Errorf("truncate purge temp table: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO `+mediaItemsPurgeTempTable+` (blob_id)
		SELECT DISTINCT media_blob_id FROM media_items WHERE user_id = ?1 AND source = ?2
	`, userID, source); err != nil {
		return 0, fmt.Errorf("stage blob ids for source=%s: %w", source, err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM media_items WHERE user_id = ?1 AND source = ?2`, userID, source); err != nil {
		return 0, fmt.Errorf("delete media_items source=%s: %w", source, err)
	}
	tag, err := tx.ExecContext(ctx, `DELETE FROM media_blobs WHERE id IN (SELECT blob_id FROM `+mediaItemsPurgeTempTable+`)`)
	if err != nil {
		return 0, fmt.Errorf("delete media_blobs source=%s: %w", source, err)
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+mediaItemsPurgeTempTable); err != nil {
		return 0, fmt.Errorf("drop purge temp table: %w", err)
	}
	return RowsAffected(tag), nil
}

// DeleteEmailAttachmentMediaBySourceRefsTx removes media_items for IMAP/Gmail email attachments
// whose source_reference is in refs, then deletes media_blobs that are no longer referenced
// by any media_item. Uses a temp table instead of DELETE-in-WITH for SQLite < 3.35.
func DeleteEmailAttachmentMediaBySourceRefsTx(ctx context.Context, tx *sql.Tx, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	refCond, refArgs, _ := StringIN("source_reference", refs, 1)
	srcCond := strings.Join([]string{
		"source IN ('email_attachment', 'gmail_attachment')",
		refCond,
	}, " AND ")

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS `+emailAttachmentBlobPurgeTempTable+` (blob_id INTEGER)`); err != nil {
		return fmt.Errorf("create email attachment purge temp table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+emailAttachmentBlobPurgeTempTable); err != nil {
		return fmt.Errorf("truncate email attachment purge temp table: %w", err)
	}
	insertQ := `
		INSERT INTO ` + emailAttachmentBlobPurgeTempTable + ` (blob_id)
		SELECT DISTINCT media_blob_id FROM media_items WHERE ` + srcCond
	if _, err := tx.ExecContext(ctx, insertQ, refArgs...); err != nil {
		return fmt.Errorf("stage email attachment blob ids: %w", err)
	}
	deleteItemsQ := `DELETE FROM media_items WHERE ` + srcCond
	if _, err := tx.ExecContext(ctx, deleteItemsQ, refArgs...); err != nil {
		return fmt.Errorf("delete email attachment media_items: %w", err)
	}
	deleteBlobsQ := `
		DELETE FROM media_blobs b
		WHERE b.id IN (SELECT blob_id FROM ` + emailAttachmentBlobPurgeTempTable + `)
		  AND NOT EXISTS (SELECT 1 FROM media_items m WHERE m.media_blob_id = b.id)`
	if _, err := tx.ExecContext(ctx, deleteBlobsQ); err != nil {
		return fmt.Errorf("delete unreferenced email attachment blobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+emailAttachmentBlobPurgeTempTable); err != nil {
		return fmt.Errorf("drop email attachment purge temp table: %w", err)
	}
	return nil
}

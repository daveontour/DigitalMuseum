package facebookall

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ClearFacebookAllData removes all data imported by Messenger, Albums, Places, and Posts.
// Uses media_blobs (digitalmuseum schema). Runs in a single transaction.
func ClearFacebookAllData(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Messenger: delete attachment media_items (cascades to message_attachments via FK)
	// and their blobs via CTE returning.
	_, err = tx.Exec(ctx, `
		WITH deleted_items AS (
			DELETE FROM media_items WHERE source = 'Facebook' RETURNING media_blob_id
		)
		DELETE FROM media_blobs WHERE id IN (SELECT media_blob_id FROM deleted_items)
	`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook Messenger media: %w", err)
	}

	// Messenger: delete messages (service = 'Facebook Messenger'), cascades to message_attachments.
	_, err = tx.Exec(ctx, `DELETE FROM messages WHERE service = 'Facebook Messenger'`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook Messenger messages: %w", err)
	}

	// Albums: delete album media_items (cascades to album_media via FK) and their blobs.
	_, err = tx.Exec(ctx, `
		WITH deleted_items AS (
			DELETE FROM media_items WHERE source = 'facebook_album' RETURNING media_blob_id
		)
		DELETE FROM media_blobs WHERE id IN (SELECT media_blob_id FROM deleted_items)
	`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook album media: %w", err)
	}

	// Albums: delete album records (cascades to any remaining album_media rows).
	_, err = tx.Exec(ctx, `DELETE FROM facebook_albums`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook albums: %w", err)
	}

	// Places.
	_, err = tx.Exec(ctx, `DELETE FROM locations WHERE source = 'facebook'`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook places: %w", err)
	}

	// Posts: delete post media_items (cascades to post_media via FK) and their blobs.
	_, err = tx.Exec(ctx, `
		WITH deleted_items AS (
			DELETE FROM media_items WHERE source = 'facebook_post' RETURNING media_blob_id
		)
		DELETE FROM media_blobs WHERE id IN (SELECT media_blob_id FROM deleted_items)
	`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook post media: %w", err)
	}

	// Posts: delete post records (cascades to any remaining post_media rows).
	_, err = tx.Exec(ctx, `DELETE FROM facebook_posts`)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook posts: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit clear transaction: %w", err)
	}
	return nil
}

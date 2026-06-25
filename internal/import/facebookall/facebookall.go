package facebookall

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ClearFacebookAllDataForUser removes Messenger, Albums, Places, and Posts data for one archive (user_id).
// Uses media_blobs (digitalmuseum schema). Runs in a single transaction.
func ClearFacebookAllDataForUser(ctx context.Context, pool *sql.DB, userID int64) error {
	if userID == 0 {
		return fmt.Errorf("user_id is required for scoped Facebook clear")
	}
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Messenger: delete attachment media_items and their blobs.
	if _, err = sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, userID, "Facebook"); err != nil {
		return fmt.Errorf("failed to clear Facebook Messenger media: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM messages WHERE service = 'Facebook Messenger' AND user_id = ?1`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook Messenger messages: %w", err)
	}

	if _, err = sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, userID, "facebook_album"); err != nil {
		return fmt.Errorf("failed to clear Facebook album media: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM facebook_albums WHERE user_id = ?1`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook albums: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM locations WHERE source = 'facebook' AND user_id = ?1`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook places: %w", err)
	}

	if _, err = sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, userID, "facebook_post"); err != nil {
		return fmt.Errorf("failed to clear Facebook post media: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM facebook_posts WHERE user_id = ?1`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear Facebook posts: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit clear transaction: %w", err)
	}
	return nil
}

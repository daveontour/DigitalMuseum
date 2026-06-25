package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/daveontour/aimuseum/internal/appctx"
	facebookallimport "github.com/daveontour/aimuseum/internal/import/facebookall"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/daveontour/aimuseum/internal/sqlutil"
	"github.com/go-chi/chi/v5"
)

// ImportDataPurgeHandler removes archive data by logical import kind (owner master unlock).
type ImportDataPurgeHandler struct {
	pool         *sql.DB
	sessionStore *keystore.SessionMasterStore
	sensitiveSvc *service.SensitiveService
	authSvc      *service.AuthService
}

// NewImportDataPurgeHandler constructs an ImportDataPurgeHandler.
func NewImportDataPurgeHandler(pool *sql.DB, sessionStore *keystore.SessionMasterStore, sensitiveSvc *service.SensitiveService, authSvc *service.AuthService) *ImportDataPurgeHandler {
	return &ImportDataPurgeHandler{pool: pool, sessionStore: sessionStore, sensitiveSvc: sensitiveSvc, authSvc: authSvc}
}

// RegisterRoutes mounts POST /api/import-data/purge.
func (h *ImportDataPurgeHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/import-data/purge", h.Purge)
}

type purgeRequest struct {
	Kind string `json:"kind"`
}

// Purge handles POST /api/import-data/purge { "kind": "..." }.
func (h *ImportDataPurgeHandler) Purge(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		writeError(w, http.StatusForbidden, "authenticated user required")
		return
	}

	var req purgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	kind := req.Kind
	if kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required")
		return
	}

	var deleted int64
	var err error
	switch kind {
	case "emails_gmail":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM emails WHERE user_id = ?1 AND source = 'gmail'`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "emails_imap":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM emails WHERE user_id = ?1 AND (source IS NULL OR source <> 'gmail')`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "whatsapp":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM messages WHERE user_id = ?1 AND service = 'WhatsApp'`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "instagram":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM messages WHERE user_id = ?1 AND service = 'Instagram'`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "imessage":
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM messages
			WHERE user_id = ?1 AND service IN ('iMessage', 'SMS', 'MMS')`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "facebook_messenger":
		var n int64
		n, err = h.purgeFacebookMessenger(ctx, uid)
		deleted = n
	case "facebook_all":
		err = facebookallimport.ClearFacebookAllDataForUser(ctx, h.pool, uid)
		if err == nil {
			deleted = 1
		}
	case "facebook_albums":
		var n int64
		n, err = h.purgeFacebookAlbums(ctx, uid)
		deleted = n
	case "facebook_posts":
		var n int64
		n, err = h.purgeFacebookPosts(ctx, uid)
		deleted = n
	case "facebook_places":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM locations WHERE user_id = ?1 AND source = 'facebook'`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "filesystem_media":
		var n int64
		n, err = h.purgeFilesystemMedia(ctx, uid)
		deleted = n
	case "thumbnails":
		tag, e := h.pool.ExecContext(ctx, `
			UPDATE media_blobs SET thumbnail_data = NULL
			WHERE id IN (SELECT media_blob_id FROM media_items WHERE user_id = ?1)
			  AND thumbnail_data IS NOT NULL`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "reference_documents":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM reference_documents WHERE user_id = ?1`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "contacts":
		tag, e := h.pool.ExecContext(ctx, `DELETE FROM contacts WHERE user_id = ?1 AND id <> 0`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "message_embeddings":
		// message_embeddings is a vec0 table keyed by rowid=messages.id and does
		// not carry user_id directly; scope deletion via the user's messages.
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM message_embeddings
			WHERE rowid IN (
				SELECT id FROM messages WHERE COALESCE(user_id, 0) = ?1
			)`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
			_, err = h.pool.ExecContext(ctx, `
				DELETE FROM message_embedding_meta
				WHERE message_id IN (
					SELECT id FROM messages WHERE COALESCE(user_id, 0) = ?1
				)`, uid)
		}
	case "email_embeddings":
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM email_embeddings
			WHERE rowid IN (
				SELECT id FROM emails WHERE COALESCE(user_id, 0) = ?1
			)`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "media_tag_embeddings":
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM media_tag_embeddings
			WHERE rowid IN (SELECT id FROM media_items WHERE COALESCE(user_id, 0) = ?1)
		`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "facebook_post_text_embeddings":
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM facebook_post_text_embeddings
			WHERE rowid IN (SELECT id FROM facebook_posts WHERE COALESCE(user_id, 0) = ?1)
		`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "facebook_album_description_embeddings":
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM facebook_album_description_embeddings
			WHERE rowid IN (SELECT id FROM facebook_albums WHERE COALESCE(user_id, 0) = ?1)
		`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	case "artefact_embeddings":
		tag, e := h.pool.ExecContext(ctx, `
			DELETE FROM artefact_embeddings
			WHERE rowid IN (SELECT id FROM artefacts WHERE COALESCE(user_id, 0) = ?1)
		`, uid)
		err = e
		if err == nil {
			deleted = sqlutil.RowsAffected(tag)
		}
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown purge kind: %s", kind))
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{"ok": true, "kind": kind, "deleted": deleted})
}

func (h *ImportDataPurgeHandler) purgeFacebookMessenger(ctx context.Context, uid int64) (int64, error) {
	tx, err := h.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, uid, "Facebook"); err != nil {
		return 0, fmt.Errorf("facebook messenger media: %w", err)
	}

	tag, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE service = 'Facebook Messenger' AND user_id = ?1`, uid)
	if err != nil {
		return 0, fmt.Errorf("facebook messenger messages: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return sqlutil.RowsAffected(tag), nil
}

func (h *ImportDataPurgeHandler) purgeFacebookAlbums(ctx context.Context, uid int64) (int64, error) {
	tx, err := h.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, uid, "facebook_album"); err != nil {
		return 0, fmt.Errorf("facebook albums media: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		DELETE FROM facebook_album_description_embeddings
		WHERE rowid IN (SELECT id FROM facebook_albums WHERE COALESCE(user_id, 0) = ?1)
	`, uid); err != nil {
		return 0, fmt.Errorf("facebook album embeddings: %w", err)
	}

	tag, err := tx.ExecContext(ctx, `DELETE FROM facebook_albums WHERE user_id = ?1`, uid)
	if err != nil {
		return 0, fmt.Errorf("facebook albums: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return sqlutil.RowsAffected(tag), nil
}

func (h *ImportDataPurgeHandler) purgeFacebookPosts(ctx context.Context, uid int64) (int64, error) {
	tx, err := h.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, uid, "facebook_post"); err != nil {
		return 0, fmt.Errorf("facebook posts media: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		DELETE FROM facebook_post_text_embeddings
		WHERE rowid IN (SELECT id FROM facebook_posts WHERE COALESCE(user_id, 0) = ?1)
	`, uid); err != nil {
		return 0, fmt.Errorf("facebook post embeddings: %w", err)
	}

	tag, err := tx.ExecContext(ctx, `DELETE FROM facebook_posts WHERE user_id = ?1`, uid)
	if err != nil {
		return 0, fmt.Errorf("facebook posts: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return sqlutil.RowsAffected(tag), nil
}

func (h *ImportDataPurgeHandler) purgeFilesystemMedia(ctx context.Context, uid int64) (int64, error) {
	tx, err := h.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	n, err := sqlutil.DeleteMediaItemsByUserAndSourceTx(ctx, tx, uid, "filesystem")
	if err != nil {
		return 0, fmt.Errorf("filesystem media: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

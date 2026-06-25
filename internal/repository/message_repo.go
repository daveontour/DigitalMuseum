package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// MessageRepo runs queries against the messages and message_attachments tables.
type MessageRepo struct {
	pool *sql.DB
}

// NewMessageRepo creates a MessageRepo backed by the given pool.
func NewMessageRepo(pool *sql.DB) *MessageRepo {
	return &MessageRepo{pool: pool}
}

// GetChatSessionRows runs the aggregation query that powers the chat-sessions endpoint.
// Returns one row per distinct chat_session, ordered by last message date DESC.
func (r *MessageRepo) GetChatSessionRows(ctx context.Context) ([]model.ChatSessionRow, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT
		    m.chat_session,
		    COUNT(DISTINCT m.id)                                                           AS message_count,
		    COUNT(DISTINCT ma.id)                                                          AS attachment_count,
		    MAX(m.service)                                                                 AS primary_service,
		    MAX(m.message_date)                                                            AS last_message_date,
		    COUNT(DISTINCT CASE WHEN m.service LIKE '%iMessage%' THEN m.id END)          AS imessage_count,
		    COUNT(DISTINCT CASE WHEN m.service LIKE '%SMS%'      THEN m.id END)          AS sms_count,
		    COUNT(DISTINCT CASE WHEN m.service = 'WhatsApp'       THEN m.id END)          AS whatsapp_count,
		    COUNT(DISTINCT CASE WHEN m.service = 'Facebook Messenger' THEN m.id END)      AS facebook_count,
		    COUNT(DISTINCT CASE WHEN m.service = 'Instagram'      THEN m.id END)          AS instagram_count,
		    COUNT(         CASE WHEN m.is_group_chat = TRUE       THEN 1    END)          AS group_chat_count
		FROM messages m
		LEFT JOIN message_attachments ma ON ma.message_id = m.id
		WHERE m.chat_session IS NOT NULL`
	args := []any{}
	// Use qualified alias — messages m and message_attachments ma both have user_id
	q, args = addUIDFilterQualified(q, args, uid, "m")
	q += `
		GROUP BY m.chat_session
		ORDER BY MAX(m.message_date) DESC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("getChatSessionRows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []model.ChatSessionRow
	for rows.Next() {
		var row model.ChatSessionRow
		if err := rows.Scan(
			&row.ChatSession,
			&row.MessageCount,
			&row.AttachmentCount,
			&row.PrimaryService,
			&row.LastMessageDate,
			&row.IMessageCount,
			&row.SMSCount,
			&row.WhatsAppCount,
			&row.FacebookCount,
			&row.InstagramCount,
			&row.GroupChatCount,
		); err != nil {
			return nil, fmt.Errorf("scan chat session row: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetConversationMessages returns all messages for a chat session ordered by message_date ASC.
func (r *MessageRepo) GetConversationMessages(ctx context.Context, chatSession string) ([]*model.Message, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id, chat_session, message_date, is_group_chat, delivered_date, read_date,
		       edited_date, service, type, sender_id, sender_name, status, replying_to,
		       subject, text, processed, created_at, updated_at
		FROM messages
		WHERE chat_session = ?1`
	args := []any{chatSession}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY message_date ASC`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("getConversationMessages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanMessages(rows)
}

// GetFirstAttachmentForMessages returns the first message_attachment record for each
// message ID in the provided set, along with the linked media_item's title and media_type.
// Returns a map of messageID → (title, mediaType).
func (r *MessageRepo) GetFirstAttachmentForMessages(ctx context.Context, messageIDs []int64) (map[int64][2]*string, error) {
	result := make(map[int64][2]*string)
	if len(messageIDs) == 0 {
		return result, nil
	}

	inCond, inArgs, _ := sqlutil.Int64IN("ma.message_id", messageIDs, 1)
	q := fmt.Sprintf(`
		SELECT message_id, title, media_type FROM (
			SELECT ma.message_id,
			       mi.title,
			       mi.media_type,
			       ROW_NUMBER() OVER (PARTITION BY ma.message_id ORDER BY ma.id ASC) AS rn
			FROM message_attachments ma
			JOIN media_items mi ON mi.id = ma.media_item_id
			WHERE %s
		) t WHERE rn = 1`, inCond)
	rows, err := r.pool.QueryContext(ctx, q, inArgs...)
	if err != nil {
		return nil, fmt.Errorf("getFirstAttachmentForMessages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var msgID int64
		var title, mediaType *string
		if err := rows.Scan(&msgID, &title, &mediaType); err != nil {
			return nil, err
		}
		result[msgID] = [2]*string{title, mediaType}
	}
	return result, rows.Err()
}

// GetMessageByID returns a single message by primary key.
func (r *MessageRepo) GetMessageByID(ctx context.Context, id int64) (*model.Message, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id, chat_session, message_date, is_group_chat, delivered_date, read_date,
		       edited_date, service, type, sender_id, sender_name, status, replying_to,
		       subject, text, processed, created_at, updated_at
		FROM messages
		WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("getMessageByID %d: %w", id, err)
	}
	defer func() { _ = rows.Close() }()

	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	return msgs[0], nil
}

// GetAttachmentMediaForMessage returns the media_item and media_blob for a message's
// first attachment. Returns nil, nil, nil if no attachment exists.
func (r *MessageRepo) GetAttachmentMediaForMessage(ctx context.Context, messageID int64) (*model.MediaItem, *model.MediaBlob, error) {
	// Get the first attachment record
	var mediaItemID int64
	err := r.pool.QueryRowContext(ctx, `
		SELECT media_item_id FROM message_attachments
		WHERE message_id = ?1
		ORDER BY id ASC
		LIMIT 1`, messageID,
	).Scan(&mediaItemID)
	if err != nil {
		if isNoRows(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("getAttachmentMediaForMessage: %w", err)
	}

	// Get media_item
	itemRows, err := r.pool.QueryContext(ctx,
		`SELECT `+mediaItemColsForMessage+` FROM media_items WHERE id = ?1`, mediaItemID)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = itemRows.Close() }()

	items, err := scanMediaItemsForMessage(itemRows)
	if err != nil || len(items) == 0 {
		return nil, nil, err
	}
	item := items[0]

	// Get blob
	blob := &model.MediaBlob{}
	err = r.pool.QueryRowContext(ctx,
		`SELECT id, image_data, thumbnail_data FROM media_blobs WHERE id = ?1`, item.MediaBlobID,
	).Scan(&blob.ID, &blob.ImageData, &blob.ThumbnailData)
	if err != nil {
		if isNoRows(err) {
			return item, nil, nil
		}
		return item, nil, fmt.Errorf("get blob: %w", err)
	}
	return item, blob, nil
}

// DeleteBySession deletes all message_attachments and messages for a chat_session in a single transaction.
// Returns the number of messages deleted.
func (r *MessageRepo) DeleteBySession(ctx context.Context, chatSession string) (int64, error) {
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("deleteBySession begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	q := `
		DELETE FROM message_attachments
		WHERE message_id IN (SELECT id FROM messages WHERE chat_session = ?1`
	args := []any{chatSession}
	// For the subquery we need uid filter inline
	if uid > 0 {
		args = append(args, uid)
		q += fmt.Sprintf(" AND user_id = ?%d", len(args))
	}
	q += ")"
	_, err = tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("deleteBySession attachments: %w", err)
	}

	dq := `DELETE FROM messages WHERE chat_session = ?1`
	dargs := []any{chatSession}
	dq, dargs = addUIDFilter(dq, dargs, uid)
	tag, err := tx.ExecContext(ctx, dq, dargs...)
	if err != nil {
		return 0, fmt.Errorf("deleteBySession messages: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("deleteBySession commit: %w", err)
	}
	return rowsAffectedOrZero(tag), nil
}

// ── scanners ──────────────────────────────────────────────────────────────────

func scanMessages(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*model.Message, error) {
	var msgs []*model.Message
	for rows.Next() {
		m := &model.Message{}
		if err := rows.Scan(
			&m.ID, &m.ChatSession, &m.MessageDate, &m.IsGroupChat,
			&m.DeliveredDate, &m.ReadDate, &m.EditedDate,
			&m.Service, &m.Type, &m.SenderID, &m.SenderName,
			&m.Status, &m.ReplyingTo, &m.Subject, &m.Text,
			&m.Processed, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// mediaItemColsForMessage is the minimal column set needed by the attachment endpoint.
const mediaItemColsForMessage = `id, media_blob_id, title, media_type, source_reference, is_referenced`

func scanMediaItemsForMessage(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*model.MediaItem, error) {
	var items []*model.MediaItem
	for rows.Next() {
		m := &model.MediaItem{}
		if err := rows.Scan(
			&m.ID, &m.MediaBlobID, &m.Title, &m.MediaType,
			&m.SourceReference, &m.IsReferenced,
		); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

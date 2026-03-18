package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MessageRepo runs queries against the messages and message_attachments tables.
type MessageRepo struct {
	pool *pgxpool.Pool
}

// NewMessageRepo creates a MessageRepo backed by the given pool.
func NewMessageRepo(pool *pgxpool.Pool) *MessageRepo {
	return &MessageRepo{pool: pool}
}

// GetChatSessionRows runs the aggregation query that powers the chat-sessions endpoint.
// Returns one row per distinct chat_session, ordered by last message date DESC.
func (r *MessageRepo) GetChatSessionRows(ctx context.Context) ([]model.ChatSessionRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
		    m.chat_session,
		    COUNT(DISTINCT m.id)                                                           AS message_count,
		    COUNT(DISTINCT ma.id)                                                          AS attachment_count,
		    MAX(m.service)                                                                 AS primary_service,
		    MAX(m.message_date)                                                            AS last_message_date,
		    COUNT(DISTINCT CASE WHEN m.service ILIKE '%iMessage%' THEN m.id END)          AS imessage_count,
		    COUNT(DISTINCT CASE WHEN m.service ILIKE '%SMS%'      THEN m.id END)          AS sms_count,
		    COUNT(DISTINCT CASE WHEN m.service = 'WhatsApp'       THEN m.id END)          AS whatsapp_count,
		    COUNT(DISTINCT CASE WHEN m.service = 'Facebook Messenger' THEN m.id END)      AS facebook_count,
		    COUNT(DISTINCT CASE WHEN m.service = 'Instagram'      THEN m.id END)          AS instagram_count,
		    COUNT(         CASE WHEN m.is_group_chat = TRUE       THEN 1    END)          AS group_chat_count
		FROM messages m
		LEFT JOIN message_attachments ma ON ma.message_id = m.id
		WHERE m.chat_session IS NOT NULL
		GROUP BY m.chat_session
		ORDER BY MAX(m.message_date) DESC`)
	if err != nil {
		return nil, fmt.Errorf("GetChatSessionRows: %w", err)
	}
	defer rows.Close()

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
	rows, err := r.pool.Query(ctx, `
		SELECT id, chat_session, message_date, is_group_chat, delivered_date, read_date,
		       edited_date, service, type, sender_id, sender_name, status, replying_to,
		       subject, text, processed, created_at, updated_at
		FROM messages
		WHERE chat_session = $1
		ORDER BY message_date ASC`, chatSession)
	if err != nil {
		return nil, fmt.Errorf("GetConversationMessages: %w", err)
	}
	defer rows.Close()
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

	// DISTINCT ON gives the first (lowest id) attachment per message
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (ma.message_id)
		    ma.message_id,
		    mi.title,
		    mi.media_type
		FROM message_attachments ma
		JOIN media_items mi ON mi.id = ma.media_item_id
		WHERE ma.message_id = ANY($1)
		ORDER BY ma.message_id, ma.id ASC`, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("GetFirstAttachmentForMessages: %w", err)
	}
	defer rows.Close()

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
	rows, err := r.pool.Query(ctx, `
		SELECT id, chat_session, message_date, is_group_chat, delivered_date, read_date,
		       edited_date, service, type, sender_id, sender_name, status, replying_to,
		       subject, text, processed, created_at, updated_at
		FROM messages
		WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("GetMessageByID %d: %w", id, err)
	}
	defer rows.Close()

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
	err := r.pool.QueryRow(ctx, `
		SELECT media_item_id FROM message_attachments
		WHERE message_id = $1
		ORDER BY id ASC
		LIMIT 1`, messageID,
	).Scan(&mediaItemID)
	if err != nil {
		if isNoRows(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("GetAttachmentMediaForMessage: %w", err)
	}

	// Get media_item
	itemRows, err := r.pool.Query(ctx,
		`SELECT `+mediaItemColsForMessage+` FROM media_items WHERE id = $1`, mediaItemID)
	if err != nil {
		return nil, nil, err
	}
	defer itemRows.Close()

	items, err := scanMediaItemsForMessage(itemRows)
	if err != nil || len(items) == 0 {
		return nil, nil, err
	}
	item := items[0]

	// Get blob
	blob := &model.MediaBlob{}
	err = r.pool.QueryRow(ctx,
		`SELECT id, image_data, thumbnail_data FROM media_blobs WHERE id = $1`, item.MediaBlobID,
	).Scan(&blob.ID, &blob.ImageData, &blob.ThumbnailData)
	if err != nil {
		if isNoRows(err) {
			return item, nil, nil
		}
		return item, nil, fmt.Errorf("get blob: %w", err)
	}
	return item, blob, nil
}

// DeleteBySession deletes all message_attachments and messages for a chat_session.
// Returns the number of messages deleted.
func (r *MessageRepo) DeleteBySession(ctx context.Context, chatSession string) (int64, error) {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM message_attachments
		WHERE message_id IN (SELECT id FROM messages WHERE chat_session = $1)`, chatSession)
	if err != nil {
		return 0, fmt.Errorf("DeleteBySession attachments: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM messages WHERE chat_session = $1`, chatSession)
	if err != nil {
		return 0, fmt.Errorf("DeleteBySession: %w", err)
	}
	return tag.RowsAffected(), nil
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

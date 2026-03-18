package importstorage

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MessageStorage handles message storage operations for imports.
type MessageStorage struct {
	pool            *pgxpool.Pool
	subjectFullName string
}

// NewMessageStorage creates a new message storage instance.
// Subject name is loaded from subject_configuration via the repo.
func NewMessageStorage(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo) *MessageStorage {
	s := &MessageStorage{pool: pool}
	if subjectRepo != nil {
		if cfg, err := subjectRepo.GetFirst(ctx); err == nil && cfg != nil {
			s.subjectFullName = cfg.SubjectName
			if cfg.FamilyName != nil && *cfg.FamilyName != "" {
				s.subjectFullName += " " + *cfg.FamilyName
			}
		}
	}
	return s
}

// GetSubjectFullName returns the subject full name used for Outgoing message rewrite.
func (s *MessageStorage) GetSubjectFullName() string {
	return s.subjectFullName
}

// MessageData represents message data for saving
type MessageData struct {
	ChatSession   *string
	MessageDate   *time.Time
	DeliveredDate *time.Time
	ReadDate      *time.Time
	EditedDate    *time.Time
	Service       *string
	Type          *string
	SenderID      *string
	SenderName    *string
	Status        *string
	ReplyingTo    *string
	Subject       *string
	Text          *string
	IsGroupChat   bool
}

// MessageWithAttachment represents a message with its attachment data
type MessageWithAttachment struct {
	MessageData        MessageData
	AttachmentData     []byte
	AttachmentFilename string
	AttachmentType     string
	Source             string
}

// BatchSaveResult contains the results of a batch save operation
type BatchSaveResult struct {
	Created                        int
	Updated                        int
	Errors                         int
	AttachmentErrorsBlobInsert     int
	AttachmentErrorsMetadataInsert int
	AttachmentErrorsJunctionInsert int
}

// SaveIMessage saves a message to the database.
// Returns the message ID and whether it was an update (true) or create (false)
func (s *MessageStorage) SaveIMessage(ctx context.Context, data MessageData, attachmentData []byte, attachmentFilename, attachmentType, source string) (int64, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, "SET LOCAL synchronous_commit = off"); err != nil {
		return 0, false, fmt.Errorf("failed to set synchronous_commit: %w", err)
	}

	var existingID int64
	var isUpdate bool

	checkQuery := `SELECT id FROM messages 
		WHERE chat_session = $1 
		AND message_date = $2 
		AND sender_id = $3 
		AND type = $4 
		LIMIT 1`

	err = tx.QueryRow(ctx, checkQuery,
		data.ChatSession,
		data.MessageDate,
		data.SenderID,
		data.Type,
	).Scan(&existingID)

	if err == nil {
		isUpdate = true
		updateQuery := `UPDATE messages SET
			delivered_date = $1,
			read_date = $2,
			edited_date = $3,
			service = $4,
			sender_name = $5,
			status = $6,
			replying_to = $7,
			subject = $8,
			text = $9,
			is_group_chat = $10,
			updated_at = NOW()
			WHERE id = $11`

		_, err = tx.Exec(ctx, updateQuery,
			data.DeliveredDate,
			data.ReadDate,
			data.EditedDate,
			data.Service,
			data.SenderName,
			data.Status,
			data.ReplyingTo,
			data.Subject,
			data.Text,
			data.IsGroupChat,
			existingID,
		)
		if err != nil {
			return 0, false, fmt.Errorf("failed to update message: %w", err)
		}

		_, err = tx.Exec(ctx, "DELETE FROM message_attachments WHERE message_id = $1", existingID)
		if err != nil {
			return 0, false, fmt.Errorf("failed to delete existing attachments: %w", err)
		}

	} else if err == pgx.ErrNoRows {
		isUpdate = false
		insertQuery := `INSERT INTO messages (
			chat_session, message_date, delivered_date, read_date, edited_date,
			service, type, sender_id, sender_name, status, replying_to,
			subject, text, is_group_chat, processed, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
		RETURNING id`

		err = tx.QueryRow(ctx, insertQuery,
			data.ChatSession,
			data.MessageDate,
			data.DeliveredDate,
			data.ReadDate,
			data.EditedDate,
			data.Service,
			data.Type,
			data.SenderID,
			data.SenderName,
			data.Status,
			data.ReplyingTo,
			data.Subject,
			data.Text,
			data.IsGroupChat,
			false,
		).Scan(&existingID)
		if err != nil {
			return 0, false, fmt.Errorf("failed to insert message: %w", err)
		}
	} else {
		return 0, false, fmt.Errorf("failed to check for existing message: %w", err)
	}

	if len(attachmentData) > 0 {
		err = s.saveAttachment(ctx, tx, existingID, attachmentData, attachmentFilename, attachmentType, source, data)
		if err != nil {
			slog.Warn("could not save attachment", "err", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return existingID, isUpdate, nil
}

func (s *MessageStorage) saveAttachment(ctx context.Context, tx pgx.Tx, messageID int64, attachmentData []byte, attachmentFilename, attachmentType, source string, messageData MessageData) error {
	var thumbnailData []byte

	var blobID int64
	insertBlobQuery := `INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, $2) RETURNING id`
	err := tx.QueryRow(ctx, insertBlobQuery, attachmentData, thumbnailData).Scan(&blobID)
	if err != nil {
		return fmt.Errorf("failed to insert media blob (filename: %s, size: %d bytes, type: %s): %w",
			attachmentFilename, len(attachmentData), attachmentType, err)
	}

	var year, month *int
	if messageData.MessageDate != nil {
		y := messageData.MessageDate.Year()
		m := int(messageData.MessageDate.Month())
		year = &y
		month = &m
	}

	if source == "" {
		source = "message_attachment"
	}

	chatSessionStr := ""
	if messageData.ChatSession != nil {
		chatSessionStr = *messageData.ChatSession
	}
	messageIDStr := fmt.Sprintf("%d", messageID)

	insertMetaQuery := `INSERT INTO media_items (
		media_blob_id, tags, source, source_reference, title, description,
		media_type, year, month, latitude, longitude, altitude, has_gps,
		processed, available_for_task, rating, is_personal, is_business,
		is_social, is_promotional, is_spam, is_important, created_at, updated_at, is_referenced
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW(), NOW(), FALSE)
	RETURNING id`

	var mediaItemID int64
	err = tx.QueryRow(ctx, insertMetaQuery,
		blobID,
		chatSessionStr,
		source,
		messageIDStr,
		attachmentFilename,
		nil,
		attachmentType,
		year,
		month,
		nil,
		nil,
		nil,
		false,
		false,
		false,
		5,
		false, false, false, false, false, false,
	).Scan(&mediaItemID)
	if err != nil {
		return fmt.Errorf("failed to insert media metadata (blob_id: %d, filename: %s, type: %s): %w",
			blobID, attachmentFilename, attachmentType, err)
	}

	insertJunctionQuery := `INSERT INTO message_attachments (message_id, media_item_id) VALUES ($1, $2)`
	_, err = tx.Exec(ctx, insertJunctionQuery, messageID, mediaItemID)
	if err != nil {
		return fmt.Errorf("failed to insert message attachment junction (message_id: %d, media_item_id: %d, filename: %s): %w",
			messageID, mediaItemID, attachmentFilename, err)
	}

	return nil
}

// SetIsGroupChat sets the is_group_chat flag for group chats
func (s *MessageStorage) SetIsGroupChat(ctx context.Context) error {
	query := `WITH GroupSessions AS (
		SELECT chat_session, service
		FROM messages
		WHERE service IN ('WhatsApp', 'Facebook Messenger')
		GROUP BY chat_session, service
		HAVING COUNT(DISTINCT sender_id) > 2
	)
	UPDATE messages m
	SET is_group_chat = TRUE
	FROM GroupSessions gs
	WHERE m.chat_session = gs.chat_session AND m.service = gs.service`

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to set is_group_chat flag: %w", err)
	}
	return nil
}

// DeleteOrphanFacebookConversations deletes messages from Facebook Messenger chats with fewer than 2 messages
func (s *MessageStorage) DeleteOrphanFacebookConversations(ctx context.Context) error {
	query := `DELETE FROM messages WHERE service = 'Facebook Messenger' AND chat_session IN (
		SELECT chat_session FROM messages WHERE service = 'Facebook Messenger'
		GROUP BY chat_session HAVING COUNT(*) < 2
	)`
	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to delete orphan Facebook conversations: %w", err)
	}
	return nil
}

// SaveMessagesBatch saves multiple messages in a single transaction using bulk operations
func (s *MessageStorage) SaveMessagesBatch(ctx context.Context, messages []MessageWithAttachment) (*BatchSaveResult, error) {
	if len(messages) == 0 {
		return &BatchSaveResult{}, nil
	}

	result := &BatchSaveResult{}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, "SET LOCAL synchronous_commit = off"); err != nil {
		return nil, fmt.Errorf("failed to set synchronous_commit: %w", err)
	}

	type messageKey struct {
		chatSession string
		messageDate time.Time
		senderID    string
		msgType     string
	}

	for i := range messages {
		if messages[i].MessageData.Type != nil && *messages[i].MessageData.Type == "Outgoing" {
			messages[i].MessageData.SenderID = &s.subjectFullName
			messages[i].MessageData.SenderName = &s.subjectFullName
		}
	}

	existingMap := make(map[messageKey]int64)
	if len(messages) > 0 {
		var checkQuery strings.Builder
		checkQuery.WriteString(`SELECT id, chat_session, message_date, sender_id, type 
			FROM messages 
			WHERE (chat_session, message_date, sender_id, type) IN (`)

		args := make([]interface{}, 0, len(messages)*4)
		placeholders := make([]string, 0, len(messages))
		argIndex := 1

		for _, msg := range messages {
			if msg.MessageData.ChatSession != nil && msg.MessageData.MessageDate != nil &&
				msg.MessageData.SenderID != nil && msg.MessageData.Type != nil {
				placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d)",
					argIndex, argIndex+1, argIndex+2, argIndex+3))
				args = append(args, *msg.MessageData.ChatSession, *msg.MessageData.MessageDate,
					*msg.MessageData.SenderID, *msg.MessageData.Type)
				argIndex += 4
			}
		}

		if len(placeholders) > 0 {
			checkQuery.WriteString(strings.Join(placeholders, ", "))
			checkQuery.WriteString(")")
			rows, err := tx.Query(ctx, checkQuery.String(), args...)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var id int64
					var chatSession string
					var messageDate time.Time
					var senderID string
					var msgType string
					if err := rows.Scan(&id, &chatSession, &messageDate, &senderID, &msgType); err == nil {
						key := messageKey{chatSession: chatSession, messageDate: messageDate, senderID: senderID, msgType: msgType}
						existingMap[key] = id
					}
				}
			}
		}
	}

	var toInsert []MessageWithAttachment
	var toUpdate []struct {
		msg MessageWithAttachment
		id  int64
	}

	for _, msg := range messages {
		if msg.MessageData.ChatSession == nil || msg.MessageData.MessageDate == nil ||
			msg.MessageData.SenderID == nil || msg.MessageData.Type == nil {
			result.Errors++
			var missingFields []string
			if msg.MessageData.ChatSession == nil {
				missingFields = append(missingFields, "ChatSession")
			}
			if msg.MessageData.MessageDate == nil {
				missingFields = append(missingFields, "MessageDate")
			}
			if msg.MessageData.SenderID == nil {
				missingFields = append(missingFields, "SenderID")
			}
			if msg.MessageData.Type == nil {
				missingFields = append(missingFields, "Type")
			}
			if len(missingFields) > 0 && result.Errors <= 10 {
				slog.Warn("skipping message with missing required fields", "fields", missingFields)
			}
			continue
		}

		key := messageKey{
			chatSession: *msg.MessageData.ChatSession,
			messageDate: *msg.MessageData.MessageDate,
			senderID:    *msg.MessageData.SenderID,
			msgType:     *msg.MessageData.Type,
		}

		if existingID, exists := existingMap[key]; exists {
			toUpdate = append(toUpdate, struct {
				msg MessageWithAttachment
				id  int64
			}{msg: msg, id: existingID})
		} else {
			toInsert = append(toInsert, msg)
		}
	}

	if len(toInsert) > 0 {
		insertedIDs, err := s.batchInsertMessages(ctx, tx, toInsert)
		if err != nil {
			return nil, fmt.Errorf("failed to batch insert messages: %w", err)
		}

		for i, msg := range toInsert {
			if len(msg.AttachmentData) > 0 && i < len(insertedIDs) {
				if err := s.saveAttachment(ctx, tx, insertedIDs[i], msg.AttachmentData,
					msg.AttachmentFilename, msg.AttachmentType, msg.Source, msg.MessageData); err != nil {
					slog.Error("failed to save attachment",
						"message_id", insertedIDs[i],
						"filename", msg.AttachmentFilename,
						"type", msg.AttachmentType,
						"err", err)
					errorTextLower := strings.ToLower(err.Error())
					if strings.Contains(errorTextLower, "failed to insert media blob") {
						result.AttachmentErrorsBlobInsert++
					} else if strings.Contains(errorTextLower, "failed to insert media metadata") {
						result.AttachmentErrorsMetadataInsert++
					} else if strings.Contains(errorTextLower, "failed to insert message attachment junction") {
						result.AttachmentErrorsJunctionInsert++
					} else {
						result.AttachmentErrorsBlobInsert++
					}
				}
			}
		}
		result.Created = len(toInsert)
	}

	if len(toUpdate) > 0 {
		if err := s.batchUpdateMessages(ctx, tx, toUpdate); err != nil {
			return nil, fmt.Errorf("failed to batch update messages: %w", err)
		}

		for _, item := range toUpdate {
			_, _ = tx.Exec(ctx, "DELETE FROM message_attachments WHERE message_id = $1", item.id)

			if len(item.msg.AttachmentData) > 0 {
				if err := s.saveAttachment(ctx, tx, item.id, item.msg.AttachmentData,
					item.msg.AttachmentFilename, item.msg.AttachmentType, item.msg.Source, item.msg.MessageData); err != nil {
					slog.Error("failed to save attachment", "message_id", item.id, "err", err)
					errorTextLower := strings.ToLower(err.Error())
					if strings.Contains(errorTextLower, "failed to insert media blob") {
						result.AttachmentErrorsBlobInsert++
					} else if strings.Contains(errorTextLower, "failed to insert media metadata") {
						result.AttachmentErrorsMetadataInsert++
					} else if strings.Contains(errorTextLower, "failed to insert message attachment junction") {
						result.AttachmentErrorsJunctionInsert++
					} else {
						result.AttachmentErrorsBlobInsert++
					}
				}
			}
		}
		result.Updated = len(toUpdate)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}

func (s *MessageStorage) batchInsertMessages(ctx context.Context, tx pgx.Tx, messages []MessageWithAttachment) ([]int64, error) {
	if len(messages) == 0 {
		return []int64{}, nil
	}

	var insertQuery strings.Builder
	insertQuery.WriteString(`INSERT INTO messages (
		chat_session, message_date, delivered_date, read_date, edited_date,
		service, type, sender_id, sender_name, status, replying_to,
		subject, text, is_group_chat, processed, created_at, updated_at
	) VALUES `)

	args := make([]interface{}, 0)
	placeholders := make([]string, 0, len(messages))
	argIndex := 1

	for _, msg := range messages {
		placeholder := fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW(), NOW())",
			argIndex, argIndex+1, argIndex+2, argIndex+3, argIndex+4, argIndex+5, argIndex+6, argIndex+7,
			argIndex+8, argIndex+9, argIndex+10, argIndex+11, argIndex+12, argIndex+13, argIndex+14)
		placeholders = append(placeholders, placeholder)

		args = append(args,
			msg.MessageData.ChatSession,
			msg.MessageData.MessageDate,
			msg.MessageData.DeliveredDate,
			msg.MessageData.ReadDate,
			msg.MessageData.EditedDate,
			msg.MessageData.Service,
			msg.MessageData.Type,
			msg.MessageData.SenderID,
			msg.MessageData.SenderName,
			msg.MessageData.Status,
			msg.MessageData.ReplyingTo,
			msg.MessageData.Subject,
			msg.MessageData.Text,
			msg.MessageData.IsGroupChat,
			false,
		)
		argIndex += 15
	}

	insertQuery.WriteString(strings.Join(placeholders, ", "))
	insertQuery.WriteString(" RETURNING id")

	rows, err := tx.Query(ctx, insertQuery.String(), args...)
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

func (s *MessageStorage) batchUpdateMessages(ctx context.Context, tx pgx.Tx, updates []struct {
	msg MessageWithAttachment
	id  int64
}) error {
	if len(updates) == 0 {
		return nil
	}

	updateQuery := `UPDATE messages SET
		delivered_date = $1,
		read_date = $2,
		edited_date = $3,
		service = $4,
		sender_name = $5,
		status = $6,
		replying_to = $7,
		subject = $8,
		text = $9,
		is_group_chat = $10,
		updated_at = NOW()
		WHERE id = $11`

	for _, item := range updates {
		_, err := tx.Exec(ctx, updateQuery,
			item.msg.MessageData.DeliveredDate,
			item.msg.MessageData.ReadDate,
			item.msg.MessageData.EditedDate,
			item.msg.MessageData.Service,
			item.msg.MessageData.SenderName,
			item.msg.MessageData.Status,
			item.msg.MessageData.ReplyingTo,
			item.msg.MessageData.Subject,
			item.msg.MessageData.Text,
			item.msg.MessageData.IsGroupChat,
			item.id,
		)
		if err != nil {
			return fmt.Errorf("failed to update message %d: %w", item.id, err)
		}
	}

	return nil
}

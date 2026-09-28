package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ChatBotRepo accesses the chatbot_conversations, chatbot_turns, and chatbot_attachments
// tables — the ChatBot feature's own history, fully separate from ChatRepo (the persona chat's
// chat_conversations/chat_turns). Mirrors ChatRepo's structure and uid-scoping conventions.
type ChatBotRepo struct {
	pool *sql.DB
}

// NewChatBotRepo creates a ChatBotRepo.
func NewChatBotRepo(pool *sql.DB) *ChatBotRepo {
	return &ChatBotRepo{pool: pool}
}

// CreateConversation inserts a new chatbot_conversations row.
func (r *ChatBotRepo) CreateConversation(ctx context.Context, title string) (*model.ChatBotConversation, error) {
	uid := uidFromCtx(ctx)
	var c model.ChatBotConversation
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO chatbot_conversations (title, user_id)
		 VALUES (?1, ?2)
		 RETURNING id, title, provider, created_at, updated_at, last_message_at`,
		title, uidVal(uid),
	).Scan(&c.ID, &c.Title, &c.Provider, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt)
	if err != nil {
		return nil, fmt.Errorf("chatbot createConversation: %w", err)
	}
	return &c, nil
}

// GetConversation returns a single conversation by ID (owner-scoped), or nil if not found.
func (r *ChatBotRepo) GetConversation(ctx context.Context, id int64) (*model.ChatBotConversation, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, title, provider, created_at, updated_at, last_message_at
	      FROM chatbot_conversations WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chatbot getConversation %d: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var c model.ChatBotConversation
		if err := rows.Scan(&c.ID, &c.Title, &c.Provider, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt); err != nil {
			return nil, err
		}
		return &c, nil
	}
	return nil, rows.Err()
}

// ListConversations returns conversations ordered by most recent activity.
func (r *ChatBotRepo) ListConversations(ctx context.Context, limit *int) ([]*model.ChatBotConversation, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, title, provider, created_at, updated_at, last_message_at
	      FROM chatbot_conversations WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY COALESCE(last_message_at, created_at) DESC"
	if limit != nil {
		args = append(args, *limit)
		q += fmt.Sprintf(" LIMIT ?%d", len(args))
	}
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chatbot listConversations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.ChatBotConversation
	for rows.Next() {
		var c model.ChatBotConversation
		if err := rows.Scan(&c.ID, &c.Title, &c.Provider, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// TurnCountsBatch returns a map of conversation_id → turn count for all given IDs in one query —
// used by the conversation rail to show a message-count preview per conversation.
func (r *ChatBotRepo) TurnCountsBatch(ctx context.Context, ids []int64) (map[int64]int64, error) {
	inCond, inArgs, _ := sqlutil.Int64IN("conversation_id", ids, 1)
	q := fmt.Sprintf(
		`SELECT conversation_id, COUNT(*) FROM chatbot_turns WHERE %s GROUP BY conversation_id`,
		inCond)
	rows, err := r.pool.QueryContext(ctx, q, inArgs...)
	if err != nil {
		return nil, fmt.Errorf("chatbot turnCountsBatch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := make(map[int64]int64, len(ids))
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("chatbot turnCountsBatch scan: %w", err)
		}
		counts[id] = n
	}
	return counts, rows.Err()
}

// UpdateConversation modifies the title.
func (r *ChatBotRepo) UpdateConversation(ctx context.Context, id int64, title *string) (*model.ChatBotConversation, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE chatbot_conversations
	      SET title = COALESCE(?1, title), updated_at = CURRENT_TIMESTAMP
	      WHERE id = ?2`
	args := []any{title, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING id, title, provider, created_at, updated_at, last_message_at`
	var c model.ChatBotConversation
	err := r.pool.QueryRowContext(ctx, q, args...).
		Scan(&c.ID, &c.Title, &c.Provider, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("chatbot updateConversation %d: %w", id, err)
	}
	return &c, nil
}

// DeleteConversation removes a conversation (cascade deletes turns and attachments).
func (r *ChatBotRepo) DeleteConversation(ctx context.Context, id int64) error {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM chatbot_conversations WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

// ClearConversationTurns deletes all turns (and their attachments, via ON DELETE CASCADE) for a
// conversation the user owns, then clears last_message_at on the conversation row.
func (r *ChatBotRepo) ClearConversationTurns(ctx context.Context, conversationID int64) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("chatbot clearConversationTurns begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	uid := uidFromCtx(ctx)
	delQ := `DELETE FROM chatbot_turns WHERE conversation_id = ?1 AND EXISTS (
		SELECT 1 FROM chatbot_conversations c WHERE c.id = ?1`
	args := []any{conversationID}
	delQ, args = addUIDFilterQualified(delQ, args, uid, "c")
	delQ += `)`
	res, err := tx.ExecContext(ctx, delQ, args...)
	if err != nil {
		return 0, fmt.Errorf("chatbot clearConversationTurns delete turns: %w", err)
	}
	n, _ := res.RowsAffected()

	// Attachments whose turn just got cascade-deleted become turn_id=NULL (ON DELETE SET NULL,
	// not CASCADE — see schema comment), so they'd otherwise linger as if newly "pending". A
	// history clear should remove them outright rather than resurrect them as attachable.
	if _, err := tx.ExecContext(ctx, `DELETE FROM chatbot_attachments WHERE conversation_id = ?1 AND turn_id IS NULL`, conversationID); err != nil {
		return 0, fmt.Errorf("chatbot clearConversationTurns delete orphaned attachments: %w", err)
	}

	updQ := `UPDATE chatbot_conversations SET last_message_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?1`
	updArgs := []any{conversationID}
	updQ, updArgs = addUIDFilter(updQ, updArgs, uid)
	if _, err := tx.ExecContext(ctx, updQ, updArgs...); err != nil {
		return 0, fmt.Errorf("chatbot clearConversationTurns update conversation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// GetTurns returns the last N turns for a conversation, in chronological order, each with its
// attachment ids batched in via a single follow-up query.
func (r *ChatBotRepo) GetTurns(ctx context.Context, conversationID int64, limit int) ([]*model.ChatBotTurn, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, conversation_id, turn_number, user_input, response_text, provider, temperature, created_at
		 FROM (
		   SELECT id, conversation_id, turn_number, user_input, response_text, provider, temperature, created_at
		   FROM chatbot_turns WHERE conversation_id = ?1
		   ORDER BY turn_number DESC LIMIT ?2
		 ) sub ORDER BY turn_number ASC`,
		conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("chatbot getTurns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.ChatBotTurn
	byID := make(map[int64]*model.ChatBotTurn)
	var ids []int64
	for rows.Next() {
		var t model.ChatBotTurn
		if err := rows.Scan(&t.ID, &t.ConversationID, &t.TurnNumber, &t.UserInput, &t.ResponseText,
			&t.Provider, &t.Temperature, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
		byID[t.ID] = &t
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}
	inCond, inArgs, _ := sqlutil.Int64IN("turn_id", ids, 1)
	attRows, err := r.pool.QueryContext(ctx,
		fmt.Sprintf(`SELECT turn_id, id FROM chatbot_attachments WHERE %s ORDER BY id`, inCond), inArgs...)
	if err != nil {
		return nil, fmt.Errorf("chatbot getTurns attachments: %w", err)
	}
	defer func() { _ = attRows.Close() }()
	for attRows.Next() {
		var turnID, attID int64
		if err := attRows.Scan(&turnID, &attID); err != nil {
			return nil, err
		}
		if t, ok := byID[turnID]; ok {
			t.AttachmentIDs = append(t.AttachmentIDs, attID)
		}
	}
	return out, attRows.Err()
}

// SaveTurn inserts a new chatbot_turns row, links any given pending attachment ids to it, and
// updates last_message_at — all in a single transaction.
func (r *ChatBotRepo) SaveTurn(ctx context.Context, conversationID int64, userInput, responseText, provider string, temperature float64, attachmentIDs []int64) (*model.ChatBotTurn, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("chatbot saveTurn begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var t model.ChatBotTurn
	err = tx.QueryRowContext(ctx,
		`INSERT INTO chatbot_turns (conversation_id, turn_number, user_input, response_text, provider, temperature)
		 VALUES (?1,
		   COALESCE((SELECT MAX(turn_number) FROM chatbot_turns WHERE conversation_id = ?1), 0) + 1,
		   ?2, ?3, ?4, ?5)
		 RETURNING id, conversation_id, turn_number, user_input, response_text, provider, temperature, created_at`,
		conversationID, userInput, responseText, provider, temperature,
	).Scan(&t.ID, &t.ConversationID, &t.TurnNumber, &t.UserInput, &t.ResponseText, &t.Provider, &t.Temperature, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("chatbot saveTurn insert: %w", err)
	}

	if len(attachmentIDs) > 0 {
		inCond, inArgs, next := sqlutil.Int64IN("id", attachmentIDs, 2)
		updQ := fmt.Sprintf(`UPDATE chatbot_attachments SET turn_id = ?1 WHERE conversation_id = ?%d AND %s`, next, inCond)
		args := append([]any{t.ID}, inArgs...)
		args = append(args, conversationID)
		if _, err := tx.ExecContext(ctx, updQ, args...); err != nil {
			return nil, fmt.Errorf("chatbot saveTurn link attachments: %w", err)
		}
		t.AttachmentIDs = attachmentIDs
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE chatbot_conversations SET last_message_at = ?1, updated_at = CURRENT_TIMESTAMP WHERE id = ?2`,
		time.Now(), conversationID); err != nil {
		return nil, fmt.Errorf("chatbot saveTurn update conversation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateAttachment inserts a new chatbot_attachments row (turn_id NULL — "pending, not yet sent").
func (r *ChatBotRepo) CreateAttachment(ctx context.Context, conversationID int64, filename, contentType, kind string, size int64, data []byte, extractedText, extractionError *string) (*model.ChatBotAttachment, error) {
	uid := uidFromCtx(ctx)
	var a model.ChatBotAttachment
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO chatbot_attachments
		   (conversation_id, filename, content_type, kind, size, data, extracted_text, extraction_error, user_id)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
		 RETURNING id, conversation_id, turn_id, filename, content_type, kind, size, extracted_text, extraction_error, created_at`,
		conversationID, filename, contentType, kind, size, data, extractedText, extractionError, uidVal(uid),
	).Scan(&a.ID, &a.ConversationID, &a.TurnID, &a.Filename, &a.ContentType, &a.Kind, &a.Size, &a.ExtractedText, &a.ExtractionError, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("chatbot createAttachment: %w", err)
	}
	return &a, nil
}

// attachmentColumns excludes `data` (the raw bytes) — only GetAttachment (which needs the bytes
// to build an image request or serve a download) selects it.
const chatbotAttachmentColumns = `id, conversation_id, turn_id, filename, content_type, kind, size, extracted_text, extraction_error, created_at`

func scanChatBotAttachment(row interface{ Scan(...any) error }) (*model.ChatBotAttachment, error) {
	var a model.ChatBotAttachment
	if err := row.Scan(&a.ID, &a.ConversationID, &a.TurnID, &a.Filename, &a.ContentType, &a.Kind, &a.Size, &a.ExtractedText, &a.ExtractionError, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAttachment returns one attachment (owner-scoped) including its raw bytes, or nil if
// missing/not owned.
func (r *ChatBotRepo) GetAttachment(ctx context.Context, id int64) (*model.ChatBotAttachment, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, conversation_id, turn_id, filename, content_type, kind, size, data, extracted_text, extraction_error, created_at
	      FROM chatbot_attachments WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	var a model.ChatBotAttachment
	err := r.pool.QueryRowContext(ctx, q, args...).
		Scan(&a.ID, &a.ConversationID, &a.TurnID, &a.Filename, &a.ContentType, &a.Kind, &a.Size, &a.Data, &a.ExtractedText, &a.ExtractionError, &a.CreatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("chatbot getAttachment %d: %w", id, err)
	}
	return &a, nil
}

// ListPendingAttachments returns attachments for a conversation not yet linked to a turn
// (turn_id IS NULL) — the "attached, not yet sent" set the UI shows as removable chips.
func (r *ChatBotRepo) ListPendingAttachments(ctx context.Context, conversationID int64) ([]*model.ChatBotAttachment, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + chatbotAttachmentColumns + ` FROM chatbot_attachments
	      WHERE conversation_id = ?1 AND turn_id IS NULL`
	args := []any{conversationID}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY id`
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("chatbot listPendingAttachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*model.ChatBotAttachment
	for rows.Next() {
		a, err := scanChatBotAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAttachment removes a pending (turn_id IS NULL) attachment the user owns. Returns false
// if no matching pending row was found (already sent, already deleted, or not owned).
func (r *ChatBotRepo) DeleteAttachment(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM chatbot_attachments WHERE id = ?1 AND turn_id IS NULL`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("chatbot deleteAttachment %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// MessageService coordinates message read and write operations.
type MessageService struct {
	repo    *repository.MessageRepo
	gemini  *appai.GeminiProvider
	billing *repository.BillingRepo
	users   *repository.UserRepo
}

// NewMessageService creates a MessageService.
func NewMessageService(repo *repository.MessageRepo) *MessageService {
	return &MessageService{repo: repo}
}

// WithGemini injects a GeminiProvider for AI summarization.
func (s *MessageService) WithGemini(g *appai.GeminiProvider) { s.gemini = g }

// WithBilling attaches the billing repo and optional user repo (for denormalized identity on billing rows).
func (s *MessageService) WithBilling(b *repository.BillingRepo, users *repository.UserRepo) {
	s.billing = b
	s.users = users
}

// GetChatSessions returns all chat sessions categorised into contacts, groups, and other.
// The categorisation logic matches the Python MessageService.get_chat_sessions exactly:
//   - Phone numbers   → other
//   - Group chats     → groups
//   - Everything else → contacts
func (s *MessageService) GetChatSessions(ctx context.Context) (model.ChatSessionsResponse, error) {
	rows, err := s.repo.GetChatSessionRows(ctx)
	if err != nil {
		return model.ChatSessionsResponse{
			Contacts: []model.ChatSessionInfo{},
			Groups:   []model.ChatSessionInfo{},
			Other:    []model.ChatSessionInfo{},
		}, fmt.Errorf("get chat session rows: %w", err)
	}

	contacts := make([]model.ChatSessionInfo, 0)
	groups := make([]model.ChatSessionInfo, 0)
	other := make([]model.ChatSessionInfo, 0)

	for _, row := range rows {
		info := model.ChatSessionInfo{
			ChatSession:     row.ChatSession,
			MessageCount:    row.MessageCount,
			HasAttachments:  row.AttachmentCount > 0,
			AttachmentCount: row.AttachmentCount,
			MessageType:     determineMessageType(row),
			LastMessageDate: row.LastMessageDate,
		}

		switch {
		case isPhoneNumber(row.ChatSession):
			other = append(other, info)
		case row.GroupChatCount > 0:
			groups = append(groups, info)
		default:
			contacts = append(contacts, info)
		}
	}

	return model.ChatSessionsResponse{
		Contacts: contacts,
		Groups:   groups,
		Other:    other,
	}, nil
}

// GetConversationMessages returns all messages for a session, with first-attachment info
// batch-fetched (no N+1). Returns an empty slice (not error) when the session has no messages.
func (s *MessageService) GetConversationMessages(ctx context.Context, chatSession string) ([]model.ConversationMessage, error) {
	msgs, err := s.repo.GetConversationMessages(ctx, chatSession)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return []model.ConversationMessage{}, nil
	}

	// Collect IDs for batch attachment lookup
	ids := make([]int64, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	attMap, err := s.repo.GetFirstAttachmentForMessages(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get attachments: %w", err)
	}

	result := make([]model.ConversationMessage, len(msgs))
	for i, m := range msgs {
		var attFilename, attType *string
		hasAtt := false
		if att, ok := attMap[m.ID]; ok {
			attFilename = att[0]
			attType = att[1]
			hasAtt = true
		}

		result[i] = model.ConversationMessage{
			ID:                 m.ID,
			ChatSession:        m.ChatSession,
			MessageDate:        isoNullTime(m.MessageDate),
			DeliveredDate:      isoNullTime(m.DeliveredDate),
			ReadDate:           isoNullTime(m.ReadDate),
			EditedDate:         isoNullTime(m.EditedDate),
			Service:            m.Service,
			Type:               m.Type,
			SenderID:           m.SenderID,
			SenderName:         m.SenderName,
			Status:             m.Status,
			ReplyingTo:         m.ReplyingTo,
			Subject:            m.Subject,
			Text:               m.Text,
			AttachmentFilename: attFilename,
			AttachmentType:     attType,
			HasAttachment:      hasAtt,
		}
	}
	return result, nil
}

// GetMessageMetadata returns a compact metadata response for a single message.
// Returns nil, nil if not found.
func (s *MessageService) GetMessageMetadata(ctx context.Context, id int64) (*model.MessageMetadataResponse, error) {
	msg, err := s.repo.GetMessageByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	resp := &model.MessageMetadataResponse{
		ID:          msg.ID,
		ChatSession: msg.ChatSession,
		MessageDate: isoNullTime(msg.MessageDate),
		Service:     msg.Service,
		Type:        msg.Type,
		SenderID:    msg.SenderID,
		SenderName:  msg.SenderName,
		Text:        msg.Text,
	}
	return resp, nil
}

// GetAttachmentContent returns binary attachment content for a message.
// preview=true → thumbnail (image/jpeg); preview=false → full image.
// Returns nil, nil if the message has no attachment.
func (s *MessageService) GetAttachmentContent(ctx context.Context, messageID int64, preview bool) (*model.ImageContent, error) {
	item, blob, err := s.repo.GetAttachmentMediaForMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil // no attachment
	}
	if blob == nil {
		return nil, fmt.Errorf("no blob") // 404
	}

	if preview {
		if len(blob.ThumbnailData) == 0 {
			return nil, fmt.Errorf("no thumbnail")
		}
		filename := "attachment"
		if item.Title != nil {
			base := strings.TrimSuffix(*item.Title, filepath.Ext(*item.Title))
			filename = base + "_thumb.jpg"
		}
		return &model.ImageContent{
			Data:        blob.ThumbnailData,
			ContentType: "image/jpeg",
			Filename:    filename,
		}, nil
	}

	if len(blob.ImageData) == 0 {
		return nil, fmt.Errorf("no content")
	}
	contentType := "application/octet-stream"
	if item.MediaType != nil && *item.MediaType != "" {
		contentType = *item.MediaType
	}
	filename := "attachment"
	if item.Title != nil && *item.Title != "" {
		filename = *item.Title
	}
	return &model.ImageContent{
		Data:        blob.ImageData,
		ContentType: contentType,
		Filename:    filename,
	}, nil
}

// DeleteConversation removes all messages and attachments for a chat session.
// Returns the number of messages deleted.
func (s *MessageService) DeleteConversation(ctx context.Context, chatSession string) (int64, error) {
	return s.repo.DeleteBySession(ctx, chatSession)
}

// conversationTranscript builds a chronological text transcript for LLM prompts.
func (s *MessageService) conversationTranscript(ctx context.Context, chatSession string) (string, error) {
	msgs, err := s.repo.GetConversationMessages(ctx, chatSession)
	if err != nil {
		return "", fmt.Errorf("get conversation: %w", err)
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("no messages found for conversation: %s", chatSession)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Conversation with %s\n\n", chatSession))
	for _, m := range msgs {
		sender := "Unknown"
		if m.SenderName != nil && *m.SenderName != "" {
			sender = *m.SenderName
		}
		date := ""
		if m.MessageDate.Valid {
			date = m.MessageDate.Time.Format("2006-01-02 15:04")
		}
		text := ""
		if m.Text != nil {
			text = *m.Text
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", date, sender, text))
	}
	return sb.String(), nil
}

// SummarizeConversation fetches a conversation and asks Gemini to summarize it.
func (s *MessageService) SummarizeConversation(ctx context.Context, chatSession string) (string, error) {
	if s.gemini == nil {
		return "", fmt.Errorf("AI summarization is not configured")
	}
	transcript, err := s.conversationTranscript(ctx, chatSession)
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`Please summarize this conversation. Include:
- Who the conversation is with
- The main topics discussed
- Key events or decisions
- The overall tone and relationship dynamic
- Date range of the conversation

Conversation:
%s`, transcript)

	result, err := s.gemini.GenerateResponse(ctx, appai.GenerateRequest{UserInput: prompt}, "", nil, nil, nil)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage("gemini", "")
		}
		MarkUsageServerKey(stub, true)
		RecordLLMUsage(ctx, s.billing, s.users, stub, err)
		return "", fmt.Errorf("AI summarize: %w", err)
	}
	MarkUsageServerKey(result.Usage, true)
	RecordLLMUsage(ctx, s.billing, s.users, result.Usage, nil)
	return result.PlainText, nil
}

// RunConversationPrompt sends the same transcript as SummarizeConversation to Gemini, prefixed with the caller-supplied instruction.
func (s *MessageService) RunConversationPrompt(ctx context.Context, chatSession, instruction string) (string, error) {
	if s.gemini == nil {
		return "", fmt.Errorf("AI is not configured")
	}
	transcript, err := s.conversationTranscript(ctx, chatSession)
	if err != nil {
		return "", err
	}
	prompt := fmt.Sprintf("%s\n\n%s", strings.TrimSpace(instruction), transcript)

	result, err := s.gemini.GenerateResponse(ctx, appai.GenerateRequest{UserInput: prompt}, "", nil, nil, nil)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage("gemini", "")
		}
		MarkUsageServerKey(stub, true)
		RecordLLMUsage(ctx, s.billing, s.users, stub, err)
		return "", fmt.Errorf("AI conversation prompt: %w", err)
	}
	MarkUsageServerKey(result.Usage, true)
	RecordLLMUsage(ctx, s.billing, s.users, result.Usage, nil)
	return result.PlainText, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// determineMessageType mirrors the Python logic for picking a service label.
// If exactly one service has non-zero count it returns that name; otherwise "mixed".
func determineMessageType(row model.ChatSessionRow) string {
	type named struct {
		count int64
		name  string
	}
	services := []named{
		{row.IMessageCount, "imessage"},
		{row.SMSCount, "sms"},
		{row.WhatsAppCount, "whatsapp"},
		{row.FacebookCount, "facebook"},
		{row.InstagramCount, "instagram"},
	}
	nonZero := 0
	last := ""
	for _, s := range services {
		if s.count > 0 {
			nonZero++
			last = s.name
		}
	}
	if nonZero == 1 {
		return last
	}
	return "mixed"
}

// isPhoneNumber mirrors the Python is_phone_number() helper exactly.
//
//   - Remove spaces, hyphens, parentheses
//   - If starts with '+': rest must be all digits and len >= 7
//   - Otherwise: digit_count >= 7 AND len(cleaned) <= 20
func isPhoneNumber(s string) bool {
	if s == "" {
		return false
	}
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' {
			return -1
		}
		return r
	}, s)

	if strings.HasPrefix(cleaned, "+") {
		rest := cleaned[1:]
		if len(rest) < 7 {
			return false
		}
		for _, c := range rest {
			if !unicode.IsDigit(c) {
				return false
			}
		}
		return true
	}

	digitCount := 0
	for _, c := range cleaned {
		if unicode.IsDigit(c) {
			digitCount++
		}
	}
	return digitCount >= 7 && len(cleaned) <= 20
}

// isoNullTime formats a NullDBTime like isoString; returns nil when invalid.
func isoNullTime(n sqlutil.NullDBTime) *string {
	if !n.Valid {
		return nil
	}
	return isoString(&n.Time)
}

// isoString formats a *time.Time as a Python-compatible isoformat string
// (no timezone suffix, matching naive datetime.isoformat() output).
// Returns nil if t is nil.
func isoString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02T15:04:05.999999")
	// Trim trailing zeros in fractional part to match Python's behaviour
	// (Python omits fractional seconds when they are zero: "2006-01-02T15:04:05")
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return &s
}

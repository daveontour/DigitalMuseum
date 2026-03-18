package model

import "time"

// Message is the domain type for a row in the messages table.
type Message struct {
	ID            int64
	ChatSession   *string
	MessageDate   *time.Time
	IsGroupChat   bool
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
	Processed     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ChatSessionRow is a single row from the chat-sessions aggregation query.
type ChatSessionRow struct {
	ChatSession     string
	MessageCount    int64
	AttachmentCount int64
	PrimaryService  *string
	LastMessageDate *time.Time
	IMessageCount   int64
	SMSCount        int64
	WhatsAppCount   int64
	FacebookCount   int64
	InstagramCount  int64
	GroupChatCount  int64
}

// ChatSessionInfo is the domain representation of a chat session for the API response.
type ChatSessionInfo struct {
	ChatSession     string     `json:"chat_session"`
	MessageCount    int64      `json:"message_count"`
	HasAttachments  bool       `json:"has_attachments"`
	AttachmentCount int64      `json:"attachment_count"`
	MessageType     string     `json:"message_type"`
	LastMessageDate *time.Time `json:"last_message_date"`
}

// ChatSessionsResponse is the shape returned by GET /imessages/chat-sessions.
type ChatSessionsResponse struct {
	Contacts []ChatSessionInfo `json:"contacts"`
	Groups   []ChatSessionInfo `json:"groups"`
	Other    []ChatSessionInfo `json:"other"`
}

// ConversationMessage is one item in the GET /imessages/conversation/{session} response.
// Dates are ISO strings (matching Python datetime.isoformat() output).
type ConversationMessage struct {
	ID                 int64   `json:"id"`
	ChatSession        *string `json:"chat_session"`
	MessageDate        *string `json:"message_date"`
	DeliveredDate      *string `json:"delivered_date"`
	ReadDate           *string `json:"read_date"`
	EditedDate         *string `json:"edited_date"`
	Service            *string `json:"service"`
	Type               *string `json:"type"`
	SenderID           *string `json:"sender_id"`
	SenderName         *string `json:"sender_name"`
	Status             *string `json:"status"`
	ReplyingTo         *string `json:"replying_to"`
	Subject            *string `json:"subject"`
	Text               *string `json:"text"`
	AttachmentFilename *string `json:"attachment_filename"`
	AttachmentType     *string `json:"attachment_type"`
	HasAttachment      bool    `json:"has_attachment"`
}

// MessageMetadataResponse is the shape returned by GET /imessages/{id}/metadata.
type MessageMetadataResponse struct {
	ID          int64   `json:"id"`
	ChatSession *string `json:"chat_session"`
	MessageDate *string `json:"message_date"`
	Service     *string `json:"service"`
	Type        *string `json:"type"`
	SenderID    *string `json:"sender_id"`
	SenderName  *string `json:"sender_name"`
	Text        *string `json:"text"`
}

// MessageAttachment is a row from message_attachments.
type MessageAttachment struct {
	ID          int64
	MessageID   int64
	MediaItemID int64
}

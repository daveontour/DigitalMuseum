package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// ChatBotConversation is a row from chatbot_conversations — the ChatBot feature's own history
// list, fully separate from chat_conversations (the persona chat's history).
type ChatBotConversation struct {
	ID            int64              `json:"id"`
	Title         string             `json:"title"`
	Provider      *string            `json:"provider,omitempty"`
	CreatedAt     sqlutil.DBTime     `json:"created_at"`
	UpdatedAt     sqlutil.DBTime     `json:"updated_at"`
	LastMessageAt sqlutil.NullDBTime `json:"last_message_at"`
}

// ChatBotTurn is a row from chatbot_turns, with the ids of any attachments sent with it.
type ChatBotTurn struct {
	ID             int64          `json:"id"`
	ConversationID int64          `json:"conversation_id"`
	TurnNumber     int            `json:"turn_number"`
	UserInput      string         `json:"user_input"`
	ResponseText   string         `json:"response_text"`
	Provider       *string        `json:"provider,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	CreatedAt      sqlutil.DBTime `json:"created_at"`
	AttachmentIDs  []int64        `json:"attachment_ids,omitempty"`
}

// ChatBotAttachment is a row from chatbot_attachments. Data is only populated when explicitly
// fetched for use (e.g. building an image request) — list/status queries omit it.
type ChatBotAttachment struct {
	ID              int64          `json:"id"`
	ConversationID  int64          `json:"conversation_id"`
	TurnID          *int64         `json:"turn_id,omitempty"`
	Filename        string         `json:"filename"`
	ContentType     string         `json:"content_type"`
	Kind            string         `json:"kind"` // "image" | "document"
	Size            int64          `json:"size"`
	Data            []byte         `json:"-"`
	ExtractedText   *string        `json:"extracted_text,omitempty"`
	ExtractionError *string        `json:"extraction_error,omitempty"`
	CreatedAt       sqlutil.DBTime `json:"created_at"`
}

// ChatBotRequest is the JSON body for POST /chatbot/generate.
type ChatBotRequest struct {
	Prompt         string   `json:"prompt"`
	ConversationID *int64   `json:"conversation_id"`
	Provider       string   `json:"provider"` // same vocabulary as ChatRequest.Provider: a key, "localai", or "auto"
	Temperature    *float64 `json:"temperature"`
	AttachmentIDs  []int64  `json:"attachment_ids,omitempty"`
}

// ChatBotResponse is the JSON response for POST /chatbot/generate.
type ChatBotResponse struct {
	Response     string         `json:"response"`
	EmbeddedJSON map[string]any `json:"embedded_json,omitempty"`
}

// ChatBotConversationCreateRequest is the JSON body for POST /chatbot/conversations.
type ChatBotConversationCreateRequest struct {
	Title string `json:"title"`
}

// ChatBotConversationUpdateRequest is the JSON body for PUT /chatbot/conversations/{id}.
type ChatBotConversationUpdateRequest struct {
	Title *string `json:"title"`
}

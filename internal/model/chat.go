package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// ChatConversation is a row from chat_conversations.
type ChatConversation struct {
	ID            int64
	Title         string
	Voice         string
	CreatedAt     sqlutil.DBTime
	UpdatedAt     sqlutil.DBTime
	LastMessageAt sqlutil.NullDBTime
}

// ChatTurn is a row from chat_turns.
type ChatTurn struct {
	ID             int64
	ConversationID int64
	TurnNumber     int
	UserInput      string
	ResponseText   string
	Voice          string
	Temperature    float64
	CreatedAt      sqlutil.DBTime
}

// ChatRequest is the JSON body for POST /chat/generate.
type ChatRequest struct {
	Prompt               string   `json:"prompt"`
	Voice                *string  `json:"voice"`
	Temperature          *float64 `json:"temperature"`
	ConversationID       *int64   `json:"conversation_id"`
	Mood                 *string  `json:"mood"`
	CompanionMode        bool     `json:"companionMode"`
	AllowExplicitContent bool     `json:"allowExplicitContent"`
	EnableSnarkiness     bool     `json:"enableSnarkiness"`
	Provider             string   `json:"provider"` // gemini, claude, deepseek, or localai
	WhosAsking           string   `json:"whos_asking"`
	RepeatQuestion       bool     `json:"repeat_question"`
}

// ChatResponse is the JSON response for POST /chat/generate.
type ChatResponse struct {
	Response     string         `json:"response"`
	Voice        string         `json:"voice"`
	EmbeddedJSON map[string]any `json:"embedded_json,omitempty"`
}

// ConversationCreateRequest is the JSON body for POST /chat/conversations.
type ConversationCreateRequest struct {
	Title string `json:"title"`
	Voice string `json:"voice"`
}

// ConversationUpdateRequest is the JSON body for PUT /chat/conversations/{id}.
type ConversationUpdateRequest struct {
	Title *string `json:"title"`
	Voice *string `json:"voice"`
}

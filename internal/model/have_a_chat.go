package model

// HaveAChatTurn is a single turn in an autonomous LLM-to-LLM conversation.
type HaveAChatTurn struct {
	Speaker string `json:"speaker"` // "claude", "gemini", or "user"
	Text    string `json:"text"`
}

// HaveAChatRequest is the body for POST /chat/have-a-chat/turn.
type HaveAChatRequest struct {
	SpeakingProvider string          `json:"speaking_provider"` // "claude" or "gemini"
	ClaudeVoice      string          `json:"claude_voice"`
	GeminiVoice      string          `json:"gemini_voice"`
	Topic            string          `json:"topic"`
	History          []HaveAChatTurn `json:"history"`
	Temperature      float64         `json:"temperature"`
}

// HaveAChatResponse is the JSON response for one LLM turn.
type HaveAChatResponse struct {
	Response     string         `json:"response"`
	Provider     string         `json:"provider"`
	Voice        string         `json:"voice"`
	EmbeddedJSON map[string]any `json:"embedded_json,omitempty"`
}

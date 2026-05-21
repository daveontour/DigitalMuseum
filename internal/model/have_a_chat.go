package model

// HaveAChatTurn is a single turn in an autonomous LLM-to-LLM conversation.
type HaveAChatTurn struct {
	Speaker string `json:"speaker"` // "a", "b", or "user"
	Text    string `json:"text"`
}

// HaveAChatRequest is the body for POST /chat/have-a-chat/turn.
type HaveAChatRequest struct {
	SpeakingSlot string `json:"speaking_slot"` // "a" or "b" — which voice speaks this turn

	VoiceA    string `json:"voice_a"`    // voice personality key for slot A
	VoiceB    string `json:"voice_b"`    // voice personality key for slot B
	ProviderA string `json:"provider_a"` // "claude", "gemini", "deepseek", or "localai" — LLM that powers voice A
	ProviderB string `json:"provider_b"` // "claude", "gemini", "deepseek", or "localai" — LLM that powers voice B

	Topic       string          `json:"topic"`
	History     []HaveAChatTurn `json:"history"`
	Temperature float64         `json:"temperature"`
	BanterMode  bool            `json:"banter_mode"`

	// AllowExplicitContent mirrors POST /chat/generate: when true, append explicit-content policy to the system prompt.
	AllowExplicitContent bool `json:"allowExplicitContent"`
}

// HaveAChatResponse is the JSON response for one LLM turn.
type HaveAChatResponse struct {
	Response     string         `json:"response"`
	Provider     string         `json:"provider"`
	Voice        string         `json:"voice"`
	EmbeddedJSON map[string]any `json:"embedded_json,omitempty"`
}

// HaveAChatSessionSave is the POST body for saving a stopped conversation.
type HaveAChatSessionSave struct {
	Topic          string          `json:"topic"`
	VoiceA         string          `json:"voice_a"`
	VoiceB         string          `json:"voice_b"`
	ProviderA      string          `json:"provider_a"`
	ProviderB      string          `json:"provider_b"`
	BanterMode     bool            `json:"banter_mode"`
	Temperature    float64         `json:"temperature"`
	AllowExplicit  bool            `json:"allow_explicit"`
	TurnCount      int             `json:"turn_count"`
	History        []HaveAChatTurn `json:"history"`
}

// HaveAChatSessionListItem is a row in GET /api/have-a-chat/sessions.
type HaveAChatSessionListItem struct {
	ID         int64  `json:"id"`
	CreatedAt  string `json:"created_at"`
	StoppedAt  string `json:"stopped_at,omitempty"`
	Topic      string `json:"topic"`
	TurnCount  int    `json:"turn_count"`
}

// HaveAChatSessionDetail is a full saved session for GET /api/have-a-chat/sessions/{id}.
type HaveAChatSessionDetail struct {
	ID            int64           `json:"id"`
	CreatedAt     string          `json:"created_at"`
	StoppedAt     string          `json:"stopped_at,omitempty"`
	Topic         string          `json:"topic"`
	VoiceA        string          `json:"voice_a"`
	VoiceB        string          `json:"voice_b"`
	ProviderA     string          `json:"provider_a"`
	ProviderB     string          `json:"provider_b"`
	BanterMode    bool            `json:"banter_mode"`
	Temperature   float64         `json:"temperature"`
	AllowExplicit bool            `json:"allow_explicit"`
	TurnCount     int             `json:"turn_count"`
	History       []HaveAChatTurn `json:"history"`
}

// HaveAChatSessionSaveResponse is returned after POST save.
type HaveAChatSessionSaveResponse struct {
	ID int64 `json:"id"`
}

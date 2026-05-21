package model

// SensitiveDataResponse is returned by the list and get endpoints.
// Details is either the decrypted string or "*****************" when not available.
type SensitiveDataResponse struct {
	ID                    int64  `json:"id"`
	Description           string `json:"description"`
	Details               string `json:"details"`
	IsPrivate             bool   `json:"is_private"`
	IsSensitive           bool   `json:"is_sensitive"`
	IncludeInSystemPrompt bool   `json:"include_in_system_prompt"`
	CreatedAt             any    `json:"created_at"`
	UpdatedAt             any    `json:"updated_at"`
}

package model

import "time"

// SensitiveData is a row from the sensitive_data table.
type SensitiveData struct {
	ID          int64
	Description string
	Details     string // encrypted base64 when stored; plaintext after decryption
	IsPrivate   bool
	IsSensitive bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SensitiveDataResponse is returned by the list and get endpoints.
// Details is either the decrypted string or "*****************" when not available.
type SensitiveDataResponse struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Details     string `json:"details"`
	IsPrivate   bool   `json:"is_private"`
	IsSensitive bool   `json:"is_sensitive"`
	CreatedAt   any    `json:"created_at"`
	UpdatedAt   any    `json:"updated_at"`
}

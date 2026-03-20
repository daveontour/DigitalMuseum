package model

import "time"

// ── Reference Documents ────────────────────────────────────────────────────────

// ReferenceDocument is a row from the reference_documents table.
type ReferenceDocument struct {
	ID               int64
	Filename         string
	Title            *string
	Description      *string
	Author           *string
	ContentType      string
	Size             int64
	Tags             *string
	Categories       *string
	Notes            *string
	AvailableForTask bool
	IsPrivate        bool
	IsSensitive      bool
	IsEncrypted      bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ── Custom Voices ──────────────────────────────────────────────────────────────

// CustomVoice is a row from the custom_voices table.
type CustomVoice struct {
	ID           int64
	Key          string
	Name         string
	Description  *string
	Instructions string
	Creativity   float64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ── Interests ──────────────────────────────────────────────────────────────────

// Interest is a row from the interests table.
type Interest struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ── Visitor key hints (non-master keyring seats) ─────────────────────────────

// VisitorKeyHint is a row from visitor_key_hints (plain-text hint for unlock UI).
// KeyringID is set when listing for admin; omitted from the public unlock-dialog JSON.
type VisitorKeyHint struct {
	ID        int64     `json:"id"`
	KeyringID int64     `json:"keyring_id,omitempty"`
	Hint      string    `json:"hint"`
	CreatedAt time.Time `json:"created_at"`
}

// ── App Configuration ──────────────────────────────────────────────────────────

// AppConfiguration is a row from the app_configuration table.
type AppConfiguration struct {
	ID          int64
	Key         string
	Value       *string
	IsMandatory bool
	Description *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ── Saved Responses ────────────────────────────────────────────────────────────

// SavedResponse is a row from the saved_responses table.
type SavedResponse struct {
	ID          int64
	Title       string
	Content     string
	Voice       *string
	LLMProvider *string
	CreatedAt   time.Time
}

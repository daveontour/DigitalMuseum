package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// ── Reference Documents ────────────────────────────────────────────────────────

// ReferenceDocument is a row from the reference_documents table.
type ReferenceDocument struct {
	ID                    int64
	Filename              string
	Title                 *string
	Description           *string
	Author                *string
	ContentType           string
	Size                  int64
	Tags                  *string
	Categories            *string
	Notes                 *string
	AvailableForTask      bool
	IncludeInSystemPrompt bool
	IsPrivate             bool
	IsSensitive           bool
	IsEncrypted           bool
	CreatedAt             sqlutil.DBTime
	UpdatedAt             sqlutil.DBTime
}

// ReferenceDocumentPromptBlob is a reference_documents row (with data blob) for system-prompt inlining.
type ReferenceDocumentPromptBlob struct {
	ID          int64
	Title       *string
	Filename    string
	ContentType string
	Data        []byte
	IsEncrypted bool
	IsSensitive bool
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
	CreatedAt    sqlutil.DBTime
	UpdatedAt    sqlutil.DBTime
}

// ── Interests ──────────────────────────────────────────────────────────────────

// Interest is a row from the interests table.
type Interest struct {
	ID        int64
	Name      string
	CreatedAt sqlutil.DBTime
	UpdatedAt sqlutil.DBTime
}

// ── Visitor key hints (non-master keyring seats) ─────────────────────────────

// VisitorKeyHint is a row from visitor_key_hints (plain-text hint for unlock UI).
// KeyringID is set when listing for admin; omitted from the public unlock-dialog JSON.
type VisitorKeyHint struct {
	ID                  int64          `json:"id"`
	KeyringID           int64          `json:"keyring_id,omitempty"`
	Hint                string         `json:"hint"`
	CreatedAt           sqlutil.DBTime `json:"created_at"`
	CanMessagesChat     bool           `json:"can_messages_chat"`
	CanEmails           bool           `json:"can_emails"`
	CanContacts         bool           `json:"can_contacts"`
	CanRelationships    bool           `json:"can_relationships"`
	CanSensitivePrivate bool           `json:"can_sensitive_private"`
	LLMAllowOwnerKeys   bool           `json:"llm_allow_owner_keys"`
	LLMAllowServerKeys  bool           `json:"llm_allow_server_keys"`
}

// ── App Configuration ──────────────────────────────────────────────────────────

// AppConfiguration is a row from the app_configuration table.
type AppConfiguration struct {
	ID          int64
	Key         string
	Value       *string
	IsMandatory bool
	Description *string
	CreatedAt   sqlutil.DBTime
	UpdatedAt   sqlutil.DBTime
}

// ── Saved Responses ────────────────────────────────────────────────────────────

// SavedResponse is a row from the saved_responses table.
type SavedResponse struct {
	ID          int64
	Title       string
	Content     string
	Voice       *string
	LLMProvider *string
	CreatedAt   sqlutil.DBTime
}

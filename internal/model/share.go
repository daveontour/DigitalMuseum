package model

import (
	"encoding/json"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// ArchiveShare represents a shareable link that gives a visitor read access to
// an archive owner's data with an optional password and tool-access policy.
type ArchiveShare struct {
	ID               string             `json:"id"`
	UserID           int64              `json:"user_id"`
	Label            *string            `json:"label,omitempty"`
	HasPassword      bool               `json:"has_password"`
	ExpiresAt        sqlutil.NullDBTime `json:"expires_at,omitempty"`
	ToolAccessPolicy json.RawMessage    `json:"tool_access_policy,omitempty"`
	CreatedAt        sqlutil.DBTime     `json:"created_at"`
}

// ArchiveSharePublic is the view returned to visitors looking up a share token.
// It intentionally omits UserID and password hash.
type ArchiveSharePublic struct {
	ID               string             `json:"id"`
	Label            *string            `json:"label,omitempty"`
	HasPassword      bool               `json:"has_password"`
	ExpiresAt        sqlutil.NullDBTime `json:"expires_at,omitempty"`
	OwnerDisplayName string             `json:"owner_display_name"`
}

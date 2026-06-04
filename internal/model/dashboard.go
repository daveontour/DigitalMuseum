package model

import (
	"database/sql"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// DashboardResponse is the shape returned by GET /api/dashboard.
type DashboardResponse struct {
	MessageCounts                   map[string]int64 `json:"message_counts"`
	TotalMessages                   int64            `json:"total_messages"`
	MessagesByYear                  map[int]int64    `json:"messages_by_year"`
	EmailsByYear                    map[int]int64    `json:"emails_by_year"`
	MessagesByContact               []ContactCount   `json:"messages_by_contact"`
	ContactsCount                   int64            `json:"contacts_count"`
	ContactsByCategory              map[string]int64 `json:"contacts_by_category"`
	TotalImages                     int64            `json:"total_images"`
	FilesystemImagesCount           int64            `json:"filesystem_images_count"`            // source=filesystem (any is_referenced)
	FilesystemImagesEmbeddedCount   int64            `json:"filesystem_images_embedded_count"`   // source=filesystem, is_referenced=false (e.g. device upload, copied scan)
	FilesystemImagesReferencedCount int64            `json:"filesystem_images_referenced_count"` // source=filesystem, is_referenced=true (path-only)
	ImportedImages                  int64            `json:"imported_images"`
	ReferenceImages                 int64            `json:"reference_images"`
	ImagesByRegion                  map[string]int64 `json:"images_by_region"`
	ThumbnailCount                  int64            `json:"thumbnail_count"`
	ThumbnailPercentage             float64          `json:"thumbnail_percentage"`
	FacebookAlbumsCount             int64            `json:"facebook_albums_count"`
	FacebookPostsCount              int64            `json:"facebook_posts_count"`
	LocationsCount                  int64            `json:"locations_count"`
	PlacesCount                     int64            `json:"places_count"`
	EmailsCount                     int64            `json:"emails_count"`
	EmailsBySource                  map[string]int64 `json:"emails_by_source"`
	ArtefactsCount                  int64            `json:"artefacts_count"`
	ReferenceDocsCount              int64            `json:"reference_docs_count"`
	ReferenceDocsEnabled            int64            `json:"reference_docs_enabled"`
	ReferenceDocsDisabled           int64            `json:"reference_docs_disabled"`
	CompleteProfilesCount           int64            `json:"complete_profiles_count"`
	SubjectFullName                 string           `json:"subject_full_name"`
	SubjectHasCompleteProfile       bool             `json:"subject_has_complete_profile"`
}

// ContactCount is a name+count pair used in MessagesByContact.
type ContactCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// ImportModalStatsResponse is the shape returned by GET /api/import-modal-stats.
// Subset of dashboard counts used by the Import & Manage Data modal.
type ImportModalStatsResponse struct {
	MessageCounts                   map[string]int64 `json:"message_counts"`
	TotalImages                     int64            `json:"total_images"`
	FilesystemImagesEmbeddedCount   int64            `json:"filesystem_images_embedded_count"`
	FilesystemImagesReferencedCount int64            `json:"filesystem_images_referenced_count"`
	ImportedImages                  int64            `json:"imported_images"`
	ReferenceImages                 int64            `json:"reference_images"`
	ThumbnailCount                  int64            `json:"thumbnail_count"`
	FacebookAlbumsCount             int64            `json:"facebook_albums_count"`
	FacebookPostsCount              int64            `json:"facebook_posts_count"`
	LocationsCount                  int64            `json:"locations_count"`
	ContactsCount                   int64            `json:"contacts_count"`
	EmailsBySource                  map[string]int64 `json:"emails_by_source"`
	ReferenceDocsCount              int64            `json:"reference_docs_count"`
	GpsImagesCount                  int64            `json:"gps_images_count"`
}

// OwnerContactSuggestion is a short contact row for linking the archive owner.
type OwnerContactSuggestion struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

// SubjectConfig is the domain type for a row in subject_configuration.
type SubjectConfig struct {
	ID                     int64
	SubjectName            string
	Gender                 string
	FamilyName             *string
	OtherNames             *string
	EmailAddresses         *string
	PhoneNumbers           *string
	WhatsAppHandle         *string
	InstagramHandle        *string
	SubjectContactID       sql.NullInt64
	WritingStyleAI         *string
	PsychologicalProfileAI *string
	CreatedAt              sqlutil.DBTime
	UpdatedAt              sqlutil.DBTime
}

// SubjectConfigResponse is the shape returned by GET /api/subject-configuration.
type SubjectConfigResponse struct {
	ID                         int64                    `json:"id"`
	SubjectName                string                   `json:"subject_name"`
	Gender                     string                   `json:"gender"`
	FamilyName                 *string                  `json:"family_name"`
	OtherNames                 *string                  `json:"other_names"`
	EmailAddresses             *string                  `json:"email_addresses"`
	PhoneNumbers               *string                  `json:"phone_numbers"`
	WhatsAppHandle             *string                  `json:"whatsapp_handle"`
	InstagramHandle            *string                  `json:"instagram_handle"`
	SubjectContactID           *int64                   `json:"subject_contact_id,omitempty"`
	LinkedOwnerContact         *OwnerContactSuggestion  `json:"linked_owner_contact,omitempty"`
	OwnerContactSuggestions    []OwnerContactSuggestion `json:"owner_contact_suggestions"`
	WritingStyleAI             *string                  `json:"writing_style_ai"`
	PsychologicalProfileAI     *string                  `json:"psychological_profile_ai"`
	SystemInstructions         string                   `json:"system_instructions"`
	CoreSystemInstructions     string                   `json:"core_system_instructions"`
	QuestionSystemInstructions string                   `json:"question_system_instructions"`
	CreatedAt                  *string                  `json:"created_at"`
	UpdatedAt                  *string                  `json:"updated_at"`
}

// DashboardRaw holds data collected by the repo before service-layer assembly.
type DashboardRaw struct {
	MessageCounts                   map[string]int64
	MessagesByYear                  map[int]int64
	EmailsByYear                    map[int]int64
	TopSenders                      []ContactCount // top 100 by count, unfiltered
	ContactsCount                   int64
	ContactsByCategory              map[string]int64
	TotalImages                     int64
	FilesystemImagesCount           int64
	FilesystemImagesEmbeddedCount   int64
	FilesystemImagesReferencedCount int64
	ImportedImages                  int64
	ReferenceImages                 int64
	ThumbnailCount                  int64
	ImagesByRegion                  map[string]int64
	FacebookAlbumsCount             int64
	FacebookPostsCount              int64
	LocationsCount                  int64
	PlacesCount                     int64
	EmailsCount                     int64
	EmailsBySource                  map[string]int64
	ArtefactsCount                  int64
	ReferenceDocsCount              int64
	ReferenceDocsEnabled            int64
	CompleteProfilesCount           int64
}

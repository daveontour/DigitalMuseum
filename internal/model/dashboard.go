package model

import "time"

// DashboardResponse is the shape returned by GET /api/dashboard.
type DashboardResponse struct {
	MessageCounts             map[string]int64 `json:"message_counts"`
	TotalMessages             int64            `json:"total_messages"`
	MessagesByYear            map[int]int64    `json:"messages_by_year"`
	EmailsByYear              map[int]int64    `json:"emails_by_year"`
	MessagesByContact         []ContactCount   `json:"messages_by_contact"`
	ContactsCount             int64            `json:"contacts_count"`
	ContactsByCategory        map[string]int64 `json:"contacts_by_category"`
	TotalImages               int64            `json:"total_images"`
	ImportedImages            int64            `json:"imported_images"`
	ReferenceImages           int64            `json:"reference_images"`
	ImagesByRegion            map[string]int64 `json:"images_by_region"`
	ThumbnailCount            int64            `json:"thumbnail_count"`
	ThumbnailPercentage       float64          `json:"thumbnail_percentage"`
	FacebookAlbumsCount       int64            `json:"facebook_albums_count"`
	FacebookPostsCount        int64            `json:"facebook_posts_count"`
	LocationsCount            int64            `json:"locations_count"`
	PlacesCount               int64            `json:"places_count"`
	EmailsCount               int64            `json:"emails_count"`
	ArtefactsCount            int64            `json:"artefacts_count"`
	ReferenceDocsCount        int64            `json:"reference_docs_count"`
	ReferenceDocsEnabled      int64            `json:"reference_docs_enabled"`
	ReferenceDocsDisabled     int64            `json:"reference_docs_disabled"`
	CompleteProfilesCount     int64            `json:"complete_profiles_count"`
	SubjectFullName           string           `json:"subject_full_name"`
	SubjectHasCompleteProfile bool             `json:"subject_has_complete_profile"`
}

// ContactCount is a name+count pair used in MessagesByContact.
type ContactCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
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
	WritingStyleAI         *string
	PsychologicalProfileAI *string
	SystemInstructions     string
	CoreSystemInstructions string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// SubjectConfigResponse is the shape returned by GET /api/subject-configuration.
type SubjectConfigResponse struct {
	ID                     int64   `json:"id"`
	SubjectName            string  `json:"subject_name"`
	Gender                 string  `json:"gender"`
	FamilyName             *string `json:"family_name"`
	OtherNames             *string `json:"other_names"`
	EmailAddresses         *string `json:"email_addresses"`
	PhoneNumbers           *string `json:"phone_numbers"`
	WhatsAppHandle         *string `json:"whatsapp_handle"`
	InstagramHandle        *string `json:"instagram_handle"`
	WritingStyleAI         *string `json:"writing_style_ai"`
	PsychologicalProfileAI *string `json:"psychological_profile_ai"`
	SystemInstructions     string  `json:"system_instructions"`
	CoreSystemInstructions string  `json:"core_system_instructions"`
	CreatedAt              *string `json:"created_at"`
	UpdatedAt              *string `json:"updated_at"`
}

// DashboardRaw holds data collected by the repo before service-layer assembly.
type DashboardRaw struct {
	MessageCounts        map[string]int64
	MessagesByYear       map[int]int64
	EmailsByYear         map[int]int64
	TopSenders           []ContactCount // top 100 by count, unfiltered
	ContactsCount        int64
	ContactsByCategory   map[string]int64
	TotalImages          int64
	ImportedImages       int64
	ReferenceImages      int64
	ThumbnailCount       int64
	ImagesByRegion       map[string]int64
	FacebookAlbumsCount  int64
	FacebookPostsCount   int64
	LocationsCount       int64
	PlacesCount          int64
	EmailsCount          int64
	ArtefactsCount       int64
	ReferenceDocsCount   int64
	ReferenceDocsEnabled int64
	CompleteProfilesCount int64
}

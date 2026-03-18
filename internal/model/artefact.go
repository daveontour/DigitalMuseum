package model

import "time"

// Artefact is a row from the artefacts table.
type Artefact struct {
	ID          int64
	Name        string
	Description *string
	Tags        *string
	Story       *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ArtefactMedia is a row from the artefact_media junction table.
type ArtefactMedia struct {
	ID          int64
	ArtefactID  int64
	MediaItemID int64
	SortOrder   int
}

// ArtefactMediaItem is the joined view used in API responses.
type ArtefactMediaItem struct {
	ID           int64   `json:"id"`           // artefact_media.id
	MediaItemID  int64   `json:"media_item_id"` // media_items.id
	MediaBlobID  *int64  `json:"media_blob_id"` // media_items.media_blob_id
	SortOrder    int     `json:"sort_order"`
	MediaType    *string `json:"media_type"`
	Title        *string `json:"title"`
	ThumbnailURL string  `json:"thumbnail_url"` // /images/{blob_id}?preview=true&type=blob
}

// ArtefactResponse is the full API response including all linked media.
type ArtefactResponse struct {
	ID          int64               `json:"id"`
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	Tags        *string             `json:"tags"`
	Story       *string             `json:"story"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	MediaItems  []ArtefactMediaItem `json:"media_items"`
}

// ArtefactSummary is the lightweight list response (date fields use time.Time for scanning).
type ArtefactSummary struct {
	ID                  int64
	Name                string
	Description         *string
	Tags                *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	PrimaryThumbnailURL *string
}

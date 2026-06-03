package model

import (
	"time"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// MediaItem is the domain type for a row in media_items.
type MediaItem struct {
	ID               int64
	MediaBlobID      int64
	Description      *string
	Title            *string
	Author           *string
	Tags             *string
	Categories       *string
	Notes            *string
	AvailableForTask bool
	MediaType        *string
	Processed        bool
	CreatedAt        sqlutil.NullDBTime
	UpdatedAt        sqlutil.NullDBTime
	Embedding        *string
	Year             *int
	Month            *int
	Day              *int
	Latitude         *float64
	Longitude        *float64
	Altitude         *float64
	Rating           int
	HasGPS           bool
	GoogleMapsURL    *string
	Region           *string
	IsPersonal       bool
	IsBusiness       bool
	IsSocial         bool
	IsPromotional    bool
	IsSpam           bool
	IsImportant      bool
	UseByAI          *bool
	IsReferenced     bool
	Source           *string
	SourceReference  *string
}

// MediaBlob holds the binary image data rows from media_blobs.
type MediaBlob struct {
	ID            int64
	ImageData     []byte
	ThumbnailData []byte
}

// MediaMetadataResponse matches the Python MediaMetadataResponse Pydantic model.
type MediaMetadataResponse struct {
	ID               int64              `json:"id"`
	MediaBlobID      int64              `json:"media_blob_id"`
	Description      *string            `json:"description"`
	Title            *string            `json:"title"`
	Author           *string            `json:"author"`
	Tags             *string            `json:"tags"`
	Categories       *string            `json:"categories"`
	Notes            *string            `json:"notes"`
	AvailableForTask bool               `json:"available_for_task"`
	MediaType        *string            `json:"media_type"`
	Processed        bool               `json:"processed"`
	CreatedAt        sqlutil.NullDBTime `json:"created_at"`
	UpdatedAt        sqlutil.NullDBTime `json:"updated_at"`
	Year             *int               `json:"year"`
	Month            *int               `json:"month"`
	Day              *int               `json:"day"`
	Latitude         *float64           `json:"latitude"`
	Longitude        *float64           `json:"longitude"`
	Altitude         *float64           `json:"altitude"`
	Rating           int                `json:"rating"`
	HasGPS           bool               `json:"has_gps"`
	GoogleMapsURL    *string            `json:"google_maps_url"`
	Region           *string            `json:"region"`
	Source           *string            `json:"source"`
	SourceReference  *string            `json:"source_reference"`
}

// ImageContent holds binary image data ready to serve as an HTTP response.
type ImageContent struct {
	Data        []byte
	ContentType string
	Filename    string
}

// ImageSearchParams holds optional filters for GET /images/search.
type ImageSearchParams struct {
	Title            *string
	Description      *string
	Author           *string
	Tags             *string // comma-separated; each tag is OR'd
	Categories       *string
	Source           *string // case-insensitive exact
	SourceReference  *string
	MediaType        *string
	Year             *int
	Month            *int
	Day              *int
	HasGPS           *bool
	HasThumbnail     *bool
	AIClassified     *bool // when true, only rows with require_classification=false
	AIUnclassified   *bool // when true, only rows with require_classification=true
	Rating           *int
	RatingMin        *int
	RatingMax        *int
	AvailableForTask *bool
	Processed        *bool
	Region           *string
	CreatedFrom      *time.Time // inclusive start date (created_at)
	CreatedTo        *time.Time // inclusive end date (created_at)
}

// GPSCountBySource is one row in GET /images/gps-count-by-source.
type GPSCountBySource struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

// GPSRegionCount is one row in GET /images/gps-regions.
type GPSRegionCount struct {
	Region string `json:"region"`
	Label  string `json:"label,omitempty"`
	Count  int64  `json:"count"`
}

// ImageRegionCount is one row in GET /images/regions.
type ImageRegionCount struct {
	Region string `json:"region"`
	Label  string `json:"label,omitempty"`
	Count  int64  `json:"count"`
}

// RandomLocationsRequest is the body for POST /getLocations/random.
type RandomLocationsRequest struct {
	Categories []string `json:"categories"`
	Limit      int      `json:"limit"`
	Region     string   `json:"region,omitempty"`
}

// NearbyLocationsRequest is the body for POST /images/locations/nearby.
type NearbyLocationsRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusKm  float64 `json:"radius_km"`
	Limit     int     `json:"limit"`
}

// BulkSetGPSRequest is the body for PUT /images/bulk-set-gps.
type BulkSetGPSRequest struct {
	ImageIDs  []int64 `json:"image_ids"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// BulkGPSUpdate is one image row updated by bulk-set-gps (coordinates may be offset on a circle).
type BulkGPSUpdate struct {
	ID        int64   `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// NearbyLocationItem is one GPS-tagged image within a radius search.
type NearbyLocationItem struct {
	LocationItem
	DistanceKm float64 `json:"distance_km"`
}

// LocationItem is the shape returned by GET /getLocations.
type LocationItem struct {
	ID              int64              `json:"id"`
	Latitude        *float64           `json:"latitude"`
	Longitude       *float64           `json:"longitude"`
	Altitude        *float64           `json:"altitude"`
	Title           *string            `json:"title"`
	Description     *string            `json:"description"`
	Year            *int               `json:"year"`
	Month           *int               `json:"month"`
	Day             *int               `json:"day"`
	Tags            *string            `json:"tags"`
	GoogleMapsURL   *string            `json:"google_maps_url"`
	Region          *string            `json:"region"`
	CreatedAt       sqlutil.NullDBTime `json:"created_at"`
	MediaType       *string            `json:"media_type"`
	Source          *string            `json:"source"`
	SourceReference *string            `json:"source_reference"`
}

// ImageMetadataJSONRecord is one row in the image metadata JSON export file.
type ImageMetadataJSONRecord struct {
	ID              int64    `json:"id"`
	SourceReference *string  `json:"source_reference"`
	Source          *string  `json:"source"`
	Latitude        *float64 `json:"latitude"`
	Longitude       *float64 `json:"longitude"`
	HasGPS          bool     `json:"has_gps"`
	GoogleMapsURL   *string  `json:"google_maps_url"`
	Tags            *string  `json:"tags"`
}

// FacebookAlbumResponse is the shape returned by GET /facebook/albums.
type FacebookAlbumResponse struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description"`
	CoverPhotoURI *string `json:"cover_photo_uri"`
	ImageCount    int     `json:"image_count"`
}

// FacebookPostListItem is the shape for a single post in GET /facebook/posts.
type FacebookPostListItem struct {
	ID          int64              `json:"id"`
	Timestamp   sqlutil.NullDBTime `json:"timestamp"`
	Title       *string            `json:"title"`
	PostText    *string            `json:"post_text"`
	ExternalURL *string            `json:"external_url"`
	PostType    *string            `json:"post_type"`
	MediaCount  int                `json:"media_count"`
}

// FacebookPostsResponse is the paginated response for GET /facebook/posts.
type FacebookPostsResponse struct {
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Posts    []FacebookPostListItem `json:"posts"`
}

// FacebookPostMediaItem is the shape for a media item in GET /facebook/posts/{id}/media.
type FacebookPostMediaItem struct {
	ID          int64   `json:"id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	MediaType   *string `json:"media_type"`
	CreatedAt   *string `json:"created_at"`
}

// FacebookPlaceItem is the shape returned by GET /facebook/places.
type FacebookPlaceItem struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Description     *string  `json:"description"`
	Address         *string  `json:"address"`
	Latitude        *float64 `json:"latitude"`
	Longitude       *float64 `json:"longitude"`
	Region          *string  `json:"region"`
	SourceReference *string  `json:"source_reference"`
}

// AlbumImageItem is the shape returned by GET /facebook/albums/{id}/images.
type AlbumImageItem struct {
	ID          int64   `json:"id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	MediaType   *string `json:"media_type"`
	CreatedAt   *string `json:"created_at"` // ISO string, matches Python .isoformat()
}

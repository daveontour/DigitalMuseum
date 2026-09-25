package model

import "github.com/daveontour/aimuseum/internal/sqlutil"

// Face is a row from media_item_faces — one detected face instance in a photo.
// BBoxX/Y/W/H are fractions (0..1) of the source image's width/height, so they
// are resolution-independent (the same values work against the full-resolution
// image and any thumbnail rendering).
//
// CreatedAt/UpdatedAt use sqlutil.DBTime, not time.Time: the Postgres-style
// schema reference declares these columns TIMESTAMP, but internal/database's
// SQLite DDL translation (pgDDLToSQLite) rewrites a bare TIMESTAMP column to
// TEXT, so the driver reports them as strings — scanning straight into
// time.Time fails. This is the same class of bug found (and fixed) in the
// message-dedup code earlier: see internal/importstorage/message_storage.go.
type Face struct {
	ID                  int64
	MediaItemID         int64
	BBoxX               float64
	BBoxY               float64
	BBoxW               float64
	BBoxH               float64
	DetectionConfidence *float64
	Landmarks           *string // JSON array of five [x,y] fractions
	EmbeddingModel      string
	FaceClusterID       *int64
	ContactID           *int64
	Ignored             bool
	// CropData is the pre-rendered JPEG crop for this face (see
	// FaceRepo.InsertFace / GetFaceCropData), generated once at detection
	// time from the full-resolution source photo already in memory then, so
	// FaceHandler.serveFaceCrop never has to re-fetch the source image or
	// re-invoke ImageMagick on every view. Deliberately left out of
	// faceColumns/scanFace (used by every other face list/read query) so
	// ordinary reads don't pull a JPEG blob per row — populated only via
	// InsertFace (write) and GetFaceCropData/SetFaceCropData (targeted read).
	CropData  []byte
	CreatedAt sqlutil.DBTime
	UpdatedAt sqlutil.DBTime
}

// FaceCluster is a row from face_clusters — a group of Face rows believed to
// be the same person. ContactID is set once the user links the cluster to an
// existing Contact; RepresentativeFaceID picks which member face's crop is
// shown as the cluster's thumbnail. SuggestedContactID/SuggestedDistance are
// the stored "possible match" guess for an unnamed cluster — written only by
// FaceService.RefreshClusterSuggestions, see there.
type FaceCluster struct {
	ID                   int64
	ContactID            *int64
	RepresentativeFaceID *int64
	FaceCount            int
	CreatedAt            sqlutil.DBTime
	UpdatedAt            sqlutil.DBTime
	SuggestedContactID   *int64
	SuggestedDistance    *float64
}

// FaceClusterWithContact is a FaceCluster joined with its linked contact's
// name (and its suggested contact's name), for list/grid responses that need
// a display label without a second round trip per cluster.
type FaceClusterWithContact struct {
	FaceCluster
	ContactName          *string
	SuggestedContactName *string
}

// ClusterSuggestion is one computed "possible match" for an unnamed cluster.
type ClusterSuggestion struct {
	ClusterID int64
	ContactID int64
	Distance  float64
}

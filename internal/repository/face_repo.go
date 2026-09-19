package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/model"
)

// FaceRepo accesses media_item_faces and face_clusters.
type FaceRepo struct {
	pool *sql.DB
}

// NewFaceRepo creates a FaceRepo backed by the given pool.
func NewFaceRepo(pool *sql.DB) *FaceRepo {
	return &FaceRepo{pool: pool}
}

const faceColumns = `id, media_item_id, bbox_x, bbox_y, bbox_w, bbox_h,
	detection_confidence, landmarks, embedding_model, face_cluster_id, contact_id, ignored,
	created_at, updated_at`

func scanFace(row interface{ Scan(...any) error }) (*model.Face, error) {
	f := &model.Face{}
	if err := row.Scan(
		&f.ID, &f.MediaItemID, &f.BBoxX, &f.BBoxY, &f.BBoxW, &f.BBoxH,
		&f.DetectionConfidence, &f.Landmarks, &f.EmbeddingModel, &f.FaceClusterID, &f.ContactID, &f.Ignored,
		&f.CreatedAt, &f.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}

// InsertFace stores one detected face for a photo. face_cluster_id and
// contact_id start unset — clustering assigns them later.
func (r *FaceRepo) InsertFace(ctx context.Context, f *model.Face) (int64, error) {
	uid := uidFromCtx(ctx)
	var id int64
	err := r.pool.QueryRowContext(ctx, `
		INSERT INTO media_item_faces (
			media_item_id, bbox_x, bbox_y, bbox_w, bbox_h,
			detection_confidence, landmarks, embedding_model, user_id, created_at, updated_at
		) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id`,
		f.MediaItemID, f.BBoxX, f.BBoxY, f.BBoxW, f.BBoxH,
		f.DetectionConfidence, f.Landmarks, f.EmbeddingModel, uidVal(uid),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert face: %w", err)
	}
	return id, nil
}

// GetFace returns one face row by id.
func (r *FaceRepo) GetFace(ctx context.Context, id int64) (*model.Face, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + faceColumns + ` FROM media_item_faces WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	return scanFace(r.pool.QueryRowContext(ctx, q, args...))
}

// ListFacesByMediaItem returns every non-ignored detected face for one
// photo, for the gallery lightbox's bounding-box overlay — an ignored face
// (typically a false detection) is excluded so no box is drawn around it.
func (r *FaceRepo) ListFacesByMediaItem(ctx context.Context, mediaItemID int64) ([]*model.Face, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + faceColumns + ` FROM media_item_faces WHERE media_item_id = ?1 AND ignored = FALSE`
	args := []any{mediaItemID}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id ASC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list faces by media item: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.Face
	for rows.Next() {
		f, err := scanFace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListFacesByCluster returns every face assigned to a cluster, for the
// cluster detail/review screen.
func (r *FaceRepo) ListFacesByCluster(ctx context.Context, clusterID int64) ([]*model.Face, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT ` + faceColumns + ` FROM media_item_faces WHERE face_cluster_id = ?1`
	args := []any{clusterID}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id ASC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list faces by cluster: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.Face
	for rows.Next() {
		f, err := scanFace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListUnclusteredFaceIDs returns face ids not yet assigned to a cluster, for
// the face-clustering background job to process.
// ListUnclusteredFaceIDs excludes ignored faces (see IgnoreFace) — an
// ignored face must never be re-homed by a later clustering run.
func (r *FaceRepo) ListUnclusteredFaceIDs(ctx context.Context) ([]int64, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id FROM media_item_faces WHERE face_cluster_id IS NULL AND ignored = FALSE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id ASC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list unclustered face ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// removeFaceFromItsCluster clears face_cluster_id/contact_id on faceID and,
// if it belonged to a cluster, decrements that cluster's face_count —
// shared by DetachFace and IgnoreFace so face_count never drifts out of
// sync with the faces actually still in a cluster. extraSet is appended to
// the face UPDATE's SET clause (e.g. "ignored = TRUE") for callers that need
// to change more than the cluster/contact assignment in the same statement.
func (r *FaceRepo) removeFaceFromItsCluster(ctx context.Context, faceID int64, extraSet string) error {
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var clusterID sql.NullInt64
	getQ := `SELECT face_cluster_id FROM media_item_faces WHERE id = ?1`
	getArgs := []any{faceID}
	getQ, getArgs = addUIDFilterDollar(getQ, getArgs, uid)
	if err := tx.QueryRowContext(ctx, getQ, getArgs...).Scan(&clusterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("face %d not found", faceID)
		}
		return fmt.Errorf("lookup face cluster: %w", err)
	}

	updateQ := `UPDATE media_item_faces SET face_cluster_id = NULL, contact_id = NULL, updated_at = CURRENT_TIMESTAMP`
	if extraSet != "" {
		updateQ += ", " + extraSet
	}
	updateQ += ` WHERE id = ?1`
	updateArgs := []any{faceID}
	updateQ, updateArgs = addUIDFilterDollar(updateQ, updateArgs, uid)
	if _, err := tx.ExecContext(ctx, updateQ, updateArgs...); err != nil {
		return fmt.Errorf("update face: %w", err)
	}

	if clusterID.Valid {
		if _, err := tx.ExecContext(ctx,
			`UPDATE face_clusters SET face_count = MAX(face_count - 1, 0), updated_at = CURRENT_TIMESTAMP WHERE id = ?1`,
			clusterID.Int64,
		); err != nil {
			return fmt.Errorf("decrement cluster face_count: %w", err)
		}
	}

	return tx.Commit()
}

// DetachFace clears a face's cluster/contact assignment (the "not this
// person" action), leaving it to be re-homed by the next clustering run.
func (r *FaceRepo) DetachFace(ctx context.Context, faceID int64) error {
	if err := r.removeFaceFromItsCluster(ctx, faceID, ""); err != nil {
		return fmt.Errorf("detach face: %w", err)
	}
	return nil
}

// IgnoreFace removes a face from its cluster (if any, decrementing its
// face_count like DetachFace) and permanently excludes it from future
// clustering runs — unlike DetachFace, an ignored face is never re-homed.
// Use this for false-positive detections or faces the user simply doesn't
// want tracked, as opposed to a real face that was just grouped wrong.
func (r *FaceRepo) IgnoreFace(ctx context.Context, faceID int64) error {
	if err := r.removeFaceFromItsCluster(ctx, faceID, "ignored = TRUE"); err != nil {
		return fmt.Errorf("ignore face: %w", err)
	}
	return nil
}

// CreateCluster creates a new cluster with the given face as its first
// (and, for now, only) member and representative face.
func (r *FaceRepo) CreateCluster(ctx context.Context, representativeFaceID int64) (int64, error) {
	uid := uidFromCtx(ctx)
	var id int64
	err := r.pool.QueryRowContext(ctx, `
		INSERT INTO face_clusters (representative_face_id, face_count, user_id, created_at, updated_at)
		VALUES (?1, 1, ?2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id`,
		representativeFaceID, uidVal(uid),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create face cluster: %w", err)
	}
	return id, nil
}

// SetNewClusterRepresentativeFace links a face to the brand-new cluster it is
// the representative of (CreateCluster already set that cluster's
// face_count=1 to account for it, so — unlike AssignFaceToCluster — this does
// not increment the count). New clusters start unnamed, so contact_id is not
// touched here.
func (r *FaceRepo) SetNewClusterRepresentativeFace(ctx context.Context, faceID, clusterID int64) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE media_item_faces SET face_cluster_id = ?1, updated_at = CURRENT_TIMESTAMP WHERE id = ?2`
	args := []any{clusterID, faceID}
	q, args = addUIDFilterDollar(q, args, uid)
	if _, err := r.pool.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("set new cluster representative face: %w", err)
	}
	return nil
}

// AssignFaceToCluster joins a face to an existing cluster, copying the
// cluster's contact_id (if any) onto the face row so contact-scoped queries
// (e.g. "find photos of X") don't need a join, and increments face_count.
func (r *FaceRepo) AssignFaceToCluster(ctx context.Context, faceID, clusterID int64) error {
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("assign face to cluster: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var contactID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT contact_id FROM face_clusters WHERE id = ?1`, clusterID).Scan(&contactID); err != nil {
		return fmt.Errorf("assign face to cluster: lookup cluster contact: %w", err)
	}

	updateQ := `UPDATE media_item_faces SET face_cluster_id = ?1, contact_id = ?2, updated_at = CURRENT_TIMESTAMP WHERE id = ?3`
	updateArgs := []any{clusterID, contactID, faceID}
	updateQ, updateArgs = addUIDFilterDollar(updateQ, updateArgs, uid)
	if _, err := tx.ExecContext(ctx, updateQ, updateArgs...); err != nil {
		return fmt.Errorf("assign face to cluster: update face: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE face_clusters SET face_count = face_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?1`, clusterID,
	); err != nil {
		return fmt.Errorf("assign face to cluster: increment count: %w", err)
	}

	return tx.Commit()
}

// SetClusterContact links a cluster to a contact and propagates contact_id
// onto every member face in one statement, so "find photos of X" queries
// never need to join through face_clusters.
func (r *FaceRepo) SetClusterContact(ctx context.Context, clusterID int64, contactID *int64) error {
	uid := uidFromCtx(ctx)
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set cluster contact: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	clusterQ := `UPDATE face_clusters SET contact_id = ?1, updated_at = CURRENT_TIMESTAMP WHERE id = ?2`
	clusterArgs := []any{contactID, clusterID}
	clusterQ, clusterArgs = addUIDFilterDollar(clusterQ, clusterArgs, uid)
	if _, err := tx.ExecContext(ctx, clusterQ, clusterArgs...); err != nil {
		return fmt.Errorf("set cluster contact: update cluster: %w", err)
	}

	facesQ := `UPDATE media_item_faces SET contact_id = ?1, updated_at = CURRENT_TIMESTAMP WHERE face_cluster_id = ?2`
	facesArgs := []any{contactID, clusterID}
	facesQ, facesArgs = addUIDFilterDollar(facesQ, facesArgs, uid)
	if _, err := tx.ExecContext(ctx, facesQ, facesArgs...); err != nil {
		return fmt.Errorf("set cluster contact: update member faces: %w", err)
	}

	return tx.Commit()
}

// GetCluster returns one cluster by id, joined with its contact's name if linked.
func (r *FaceRepo) GetCluster(ctx context.Context, id int64) (*model.FaceClusterWithContact, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT fc.id, fc.contact_id, fc.representative_face_id, fc.face_count,
		       fc.created_at, fc.updated_at, c.name
		FROM face_clusters fc
		LEFT JOIN contacts c ON c.id = fc.contact_id
		WHERE fc.id = ?1`
	args := []any{id}
	q, args = addUIDFilterQualified(q, args, uid, "fc")

	c := &model.FaceClusterWithContact{}
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(
		&c.ID, &c.ContactID, &c.RepresentativeFaceID, &c.FaceCount,
		&c.CreatedAt, &c.UpdatedAt, &c.ContactName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get face cluster: %w", err)
	}
	return c, nil
}

// ListClusters returns one page of clusters ordered by most faces first,
// optionally filtered to only named (contact_id set) or only unnamed
// clusters and/or to clusters with more than one member face (minFaceCount),
// plus the total count matching the filter (for page-count UI). A cluster
// whose faces were all detached/ignored (face_count reaching 0 — see
// removeFaceFromItsCluster) is excluded rather than shown as an empty card.
//
// Pagination is server-side (not just client-side render batching, unlike
// e.g. the main image gallery's infinite-scroll) because each thumbnail is
// an on-demand ImageMagick crop (see ClusterThumbnail/CropFaceJPEG) rather
// than a stored, cheap-to-serve file — fetching every cluster at once would
// mean firing that subprocess once per cluster in the whole archive.
// limit<=0 means "no limit" (every matching cluster, offset still applied),
// matching ContactRepo.ListShort's convention. minFaceCount<=1 applies no
// extra filter beyond the always-on face_count > 0.
func (r *FaceRepo) ListClusters(ctx context.Context, namedOnly, unnamedOnly bool, minFaceCount, limit, offset int) ([]*model.FaceClusterWithContact, int, error) {
	uid := uidFromCtx(ctx)
	var conds []string
	var args []any
	conds = append(conds, "fc.face_count > 0")
	if namedOnly {
		conds = append(conds, "fc.contact_id IS NOT NULL")
	}
	if unnamedOnly {
		conds = append(conds, "fc.contact_id IS NULL")
	}
	if minFaceCount > 1 {
		conds = append(conds, "fc.face_count >= ?")
		args = append(args, minFaceCount)
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	countQ := `SELECT COUNT(*) FROM face_clusters fc` + where
	countQ, countArgs := addUIDFilterQualified(countQ, args, uid, "fc")
	var total int
	if err := r.pool.QueryRowContext(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count face clusters: %w", err)
	}

	q := `
		SELECT fc.id, fc.contact_id, fc.representative_face_id, fc.face_count,
		       fc.created_at, fc.updated_at, c.name
		FROM face_clusters fc
		LEFT JOIN contacts c ON c.id = fc.contact_id` + where
	q, args = addUIDFilterQualified(q, args, uid, "fc")
	q += " ORDER BY fc.face_count DESC, fc.id ASC"
	if limit > 0 {
		args = append(args, limit)
		q += " LIMIT ?"
	}
	if offset > 0 {
		args = append(args, offset)
		q += " OFFSET ?"
	}

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list face clusters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.FaceClusterWithContact
	for rows.Next() {
		c := &model.FaceClusterWithContact{}
		if err := rows.Scan(
			&c.ID, &c.ContactID, &c.RepresentativeFaceID, &c.FaceCount,
			&c.CreatedAt, &c.UpdatedAt, &c.ContactName,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListImageIDsForFaceDetection returns image media_items IDs not yet scanned
// for faces (faces_processed = false), mirroring ImageRepo.ListImageIDsRequireClassification.
func (r *FaceRepo) ListImageIDsForFaceDetection(ctx context.Context) ([]int64, error) {
	uid := uidFromCtx(ctx)
	q := `
		SELECT id
		FROM media_items
		WHERE media_type LIKE 'image/%'
		  AND faces_processed = FALSE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY id ASC"

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list image ids for face detection: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkMediaItemFacesProcessed sets faces_processed = true for a photo, even
// when zero faces were found, so it isn't rescanned on every job run.
func (r *FaceRepo) MarkMediaItemFacesProcessed(ctx context.Context, mediaItemID int64) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE media_items SET faces_processed = TRUE WHERE id = ?1`
	args := []any{mediaItemID}
	q, args = addUIDFilterDollar(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark media item faces processed: %w", err)
	}
	return nil
}

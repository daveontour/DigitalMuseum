package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
)

// ArchiveShareRepo accesses the archive_shares table.
type ArchiveShareRepo struct {
	pool *sql.DB
}

// NewArchiveShareRepo creates an ArchiveShareRepo.
func NewArchiveShareRepo(pool *sql.DB) *ArchiveShareRepo {
	return &ArchiveShareRepo{pool: pool}
}

// GetByID returns the share with the given token ID, or nil if not found.
func (r *ArchiveShareRepo) GetByID(ctx context.Context, id string) (*model.ArchiveShare, error) {
	var share model.ArchiveShare
	var passwordHash *string
	var policyJSON []byte
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, user_id, label, password_hash, expires_at, tool_access_policy, created_at
		 FROM archive_shares WHERE id = ?1`, id,
	).Scan(&share.ID, &share.UserID, &share.Label, &passwordHash, &share.ExpiresAt, &policyJSON, &share.CreatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	share.HasPassword = passwordHash != nil && *passwordHash != ""
	if len(policyJSON) > 0 {
		share.ToolAccessPolicy = json.RawMessage(policyJSON)
	}
	return &share, nil
}

// GetByIDWithHash returns the share plus the raw password hash (for verification).
func (r *ArchiveShareRepo) GetByIDWithHash(ctx context.Context, id string) (*model.ArchiveShare, string, error) {
	var share model.ArchiveShare
	var passwordHash *string
	var policyJSON []byte
	err := r.pool.QueryRowContext(ctx,
		`SELECT id, user_id, label, password_hash, expires_at, tool_access_policy, created_at
		 FROM archive_shares WHERE id = ?1`, id,
	).Scan(&share.ID, &share.UserID, &share.Label, &passwordHash, &share.ExpiresAt, &policyJSON, &share.CreatedAt)
	if err != nil {
		if isNoRows(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	share.HasPassword = passwordHash != nil && *passwordHash != ""
	if len(policyJSON) > 0 {
		share.ToolAccessPolicy = json.RawMessage(policyJSON)
	}
	hash := ""
	if passwordHash != nil {
		hash = *passwordHash
	}
	return &share, hash, nil
}

// ListByUser returns all shares owned by the given user, newest first.
func (r *ArchiveShareRepo) ListByUser(ctx context.Context, userID int64) ([]*model.ArchiveShare, error) {
	rows, err := r.pool.QueryContext(ctx,
		`SELECT id, user_id, label, password_hash, expires_at, tool_access_policy, created_at
		 FROM archive_shares WHERE user_id = ?1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*model.ArchiveShare
	for rows.Next() {
		var share model.ArchiveShare
		var passwordHash *string
		var policyJSON []byte
		if err := rows.Scan(&share.ID, &share.UserID, &share.Label, &passwordHash, &share.ExpiresAt, &policyJSON, &share.CreatedAt); err != nil {
			return nil, err
		}
		share.HasPassword = passwordHash != nil && *passwordHash != ""
		if len(policyJSON) > 0 {
			share.ToolAccessPolicy = json.RawMessage(policyJSON)
		}
		out = append(out, &share)
	}
	return out, rows.Err()
}

// Create inserts a new share token.
func (r *ArchiveShareRepo) Create(ctx context.Context, id string, userID int64, label *string, passwordHash *string, expiresAt *time.Time, toolPolicy []byte) (*model.ArchiveShare, error) {
	var share model.ArchiveShare
	var ph *string
	var policyJSON []byte
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO archive_shares (id, user_id, label, password_hash, expires_at, tool_access_policy)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6)
		 RETURNING id, user_id, label, password_hash, expires_at, tool_access_policy, created_at`,
		id, userID, label, passwordHash, expiresAt, toolPolicy,
	).Scan(&share.ID, &share.UserID, &share.Label, &ph, &share.ExpiresAt, &policyJSON, &share.CreatedAt)
	if err != nil {
		return nil, err
	}
	share.HasPassword = ph != nil && *ph != ""
	if len(policyJSON) > 0 {
		share.ToolAccessPolicy = json.RawMessage(policyJSON)
	}
	return &share, nil
}

// Delete removes a share token, scoped to the owning user.
func (r *ArchiveShareRepo) Delete(ctx context.Context, id string, userID int64) error {
	tag, err := r.pool.ExecContext(ctx,
		`DELETE FROM archive_shares WHERE id = ?1 AND user_id = ?2`, id, userID)
	if err != nil {
		return err
	}
	if rowsAffectedOrZero(tag) == 0 {
		return nil // already gone
	}
	return nil
}

// GetOwnerDisplayName returns the display_name of the share's owner.
func (r *ArchiveShareRepo) GetOwnerDisplayName(ctx context.Context, userID int64) (string, error) {
	var name *string
	err := r.pool.QueryRowContext(ctx,
		`SELECT display_name FROM users WHERE id = ?1`, userID).Scan(&name)
	if err != nil {
		if isNoRows(err) {
			return "", nil
		}
		return "", err
	}
	if name != nil {
		return *name, nil
	}
	return "", nil
}

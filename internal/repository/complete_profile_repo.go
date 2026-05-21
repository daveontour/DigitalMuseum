package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CompleteProfileRepo accesses the complete_profiles table.
type CompleteProfileRepo struct {
	pool *sql.DB
}

// NewCompleteProfileRepo creates a CompleteProfileRepo.
func NewCompleteProfileRepo(pool *sql.DB) *CompleteProfileRepo {
	return &CompleteProfileRepo{pool: pool}
}

// ProfileListEntry is one row for GET /chat/complete-profile/names.
type ProfileListEntry struct {
	Name    string
	Pending bool
}

// ListProfileEntries returns all profile rows (including in-progress generations).
func (r *CompleteProfileRepo) ListProfileEntries(ctx context.Context) ([]ProfileListEntry, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT name, generation_pending FROM complete_profiles WHERE name IS NOT NULL AND name != ''`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY name"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list complete profile entries: %w", err)
	}
	defer rows.Close()
	var out []ProfileListEntry
	for rows.Next() {
		var e ProfileListEntry
		if err := rows.Scan(&e.Name, &e.Pending); err != nil {
			return nil, fmt.Errorf("scan profile entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EnqueuePendingGeneration inserts or updates a row so the name appears immediately while generation runs.
func (r *CompleteProfileRepo) EnqueuePendingGeneration(ctx context.Context, name string) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE complete_profiles SET generation_pending = TRUE, profile = '', updated_at = CURRENT_TIMESTAMP WHERE name = ?1`
	args := []any{name}
	q, args = addUIDFilter(q, args, uid)
	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("enqueue pending profile update: %w", err)
	}
	if rowsAffectedOrZero(res) > 0 {
		return nil
	}
	_, err = r.pool.ExecContext(ctx,
		`INSERT INTO complete_profiles (name, profile, user_id, generation_pending) VALUES (?1, '', ?2, TRUE)`,
		name, uidVal(uid),
	)
	if err != nil {
		return fmt.Errorf("enqueue pending profile insert: %w", err)
	}
	return nil
}

// MarkGenerationFailed clears pending state and stores a short error message in profile.
func (r *CompleteProfileRepo) MarkGenerationFailed(ctx context.Context, name string, errMsg string) error {
	uid := uidFromCtx(ctx)
	msg := "Profile generation failed. Please try again."
	if strings.TrimSpace(errMsg) != "" {
		t := strings.TrimSpace(errMsg)
		if len(t) > 800 {
			t = t[:800] + "…"
		}
		msg = "Profile generation failed: " + t
	}
	q := `UPDATE complete_profiles SET generation_pending = FALSE, profile = ?2, updated_at = CURRENT_TIMESTAMP WHERE name = ?1`
	args := []any{name, msg}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mark generation failed: %w", err)
	}
	return nil
}

// GetByName returns the profile text and whether generation is still in progress.
// If no row exists, returns found=false.
func (r *CompleteProfileRepo) GetByName(ctx context.Context, name string) (found bool, profile *string, pending bool, err error) {
	uid := uidFromCtx(ctx)
	q := `SELECT profile, generation_pending FROM complete_profiles WHERE name = ?1`
	args := []any{name}
	q, args = addUIDFilter(q, args, uid)
	var prof *string
	var pend bool
	e := r.pool.QueryRowContext(ctx, q, args...).Scan(&prof, &pend)
	if e != nil {
		if isNoRows(e) {
			return false, nil, false, nil
		}
		return false, nil, false, fmt.Errorf("get complete profile by name: %w", e)
	}
	return true, prof, pend, nil
}

// Upsert creates or updates a complete profile by name and clears generation_pending.
func (r *CompleteProfileRepo) Upsert(ctx context.Context, name, profile string) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE complete_profiles SET profile = ?2, generation_pending = FALSE, updated_at = CURRENT_TIMESTAMP WHERE name = ?1`
	args := []any{name, profile}
	q, args = addUIDFilter(q, args, uid)
	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update complete profile: %w", err)
	}
	if rowsAffectedOrZero(res) > 0 {
		return nil
	}
	_, err = r.pool.ExecContext(ctx,
		`INSERT INTO complete_profiles (name, profile, user_id, generation_pending) VALUES (?1, ?2, ?3, FALSE)`,
		name, profile, uidVal(uid),
	)
	if err != nil {
		return fmt.Errorf("insert complete profile: %w", err)
	}
	return nil
}

// DeleteByName deletes a complete profile by name. Returns true if a row was deleted.
func (r *CompleteProfileRepo) DeleteByName(ctx context.Context, name string) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM complete_profiles WHERE name = ?1`
	args := []any{name}
	q, args = addUIDFilter(q, args, uid)
	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("delete complete profile: %w", err)
	}
	return rowsAffectedOrZero(res) > 0, nil
}

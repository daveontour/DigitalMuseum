package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// PathEquivalenceRepo accesses filesystem_path_equivalences — user-defined
// rules that two directory paths should be treated as the same content for
// filesystem-import duplicate detection (see internal/import/filesystem's
// ExpandEquivalentPaths, which is the actual matching logic; this repo only
// backs the management CRUD API for the settings UI).
type PathEquivalenceRepo struct {
	pool *sql.DB
}

// NewPathEquivalenceRepo creates a PathEquivalenceRepo.
func NewPathEquivalenceRepo(pool *sql.DB) *PathEquivalenceRepo {
	return &PathEquivalenceRepo{pool: pool}
}

// List returns every path-equivalence rule for the current user, ordered by id.
func (r *PathEquivalenceRepo) List(ctx context.Context) ([]*model.PathEquivalence, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, path_a, path_b FROM filesystem_path_equivalences WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += ` ORDER BY id ASC`

	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list path equivalences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.PathEquivalence
	for rows.Next() {
		p := &model.PathEquivalence{}
		if err := rows.Scan(&p.ID, &p.PathA, &p.PathB); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Create inserts a new path-equivalence rule.
func (r *PathEquivalenceRepo) Create(ctx context.Context, pathA, pathB string) (*model.PathEquivalence, error) {
	uid := uidFromCtx(ctx)
	var id int64
	err := r.pool.QueryRowContext(ctx, `
		INSERT INTO filesystem_path_equivalences (path_a, path_b, user_id, created_at, updated_at)
		VALUES (?1, ?2, ?3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id`,
		pathA, pathB, uidVal(uid),
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create path equivalence: %w", err)
	}
	return &model.PathEquivalence{ID: id, PathA: pathA, PathB: pathB}, nil
}

// Update changes an existing rule's paths. Returns (nil, nil) when not found.
func (r *PathEquivalenceRepo) Update(ctx context.Context, id int64, pathA, pathB string) (*model.PathEquivalence, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE filesystem_path_equivalences SET path_a = ?1, path_b = ?2, updated_at = CURRENT_TIMESTAMP WHERE id = ?3`
	args := []any{pathA, pathB, id}
	q, args = addUIDFilterDollar(q, args, uid)

	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("update path equivalence: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update path equivalence: %w", err)
	}
	if n == 0 {
		return nil, nil
	}
	return &model.PathEquivalence{ID: id, PathA: pathA, PathB: pathB}, nil
}

// Delete removes a rule by id. Returns false when not found.
func (r *PathEquivalenceRepo) Delete(ctx context.Context, id int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM filesystem_path_equivalences WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilterDollar(q, args, uid)

	res, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("delete path equivalence: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete path equivalence: %w", err)
	}
	return n > 0, nil
}

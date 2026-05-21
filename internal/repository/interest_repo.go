package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/daveontour/aimuseum/internal/model"
)

// InterestRepo accesses the interests table.
type InterestRepo struct {
	pool *sql.DB
}

// NewInterestRepo creates an InterestRepo.
func NewInterestRepo(pool *sql.DB) *InterestRepo {
	return &InterestRepo{pool: pool}
}

func scanInterest(row interface{ Scan(...any) error }) (*model.Interest, error) {
	var i model.Interest
	err := row.Scan(&i.ID, &i.Name, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// List returns all interests ordered by name.
func (r *InterestRepo) List(ctx context.Context) ([]*model.Interest, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, name, created_at, updated_at FROM interests WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY name"
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListInterests: %w", err)
	}
	defer rows.Close()
	var out []*model.Interest
	for rows.Next() {
		i, err := scanInterest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// GetByID returns a single interest.
func (r *InterestRepo) GetByID(ctx context.Context, id int64) (*model.Interest, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, name, created_at, updated_at FROM interests WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	i, err := scanInterest(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return i, nil
}

// GetByName returns an interest by exact name (for uniqueness check).
func (r *InterestRepo) GetByName(ctx context.Context, name string) (*model.Interest, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, name, created_at, updated_at FROM interests WHERE name = ?1`
	args := []any{name}
	q, args = addUIDFilter(q, args, uid)
	i, err := scanInterest(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return i, nil
}

// NameExistsExcluding returns true if another row with name exists (excluding given ID).
func (r *InterestRepo) NameExistsExcluding(ctx context.Context, name string, excludeID int64) (bool, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT COUNT(*) FROM interests WHERE name=?1 AND id!=?2`
	args := []any{name, excludeID}
	q, args = addUIDFilter(q, args, uid)
	var n int
	err := r.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n > 0, err
}

// Create inserts a new interest.
func (r *InterestRepo) Create(ctx context.Context, name string) (*model.Interest, error) {
	uid := uidFromCtx(ctx)
	i, err := scanInterest(r.pool.QueryRowContext(ctx,
		`INSERT INTO interests (name, user_id) VALUES (?1, ?2)
		 RETURNING id, name, created_at, updated_at`, name, uidVal(uid)))
	if err != nil {
		return nil, fmt.Errorf("CreateInterest: %w", err)
	}
	return i, nil
}

// Update modifies an interest name.
func (r *InterestRepo) Update(ctx context.Context, id int64, name string) (*model.Interest, error) {
	uid := uidFromCtx(ctx)
	q := `UPDATE interests SET name=?1, updated_at=CURRENT_TIMESTAMP WHERE id=?2`
	args := []any{name, id}
	q, args = addUIDFilter(q, args, uid)
	q += ` RETURNING id, name, created_at, updated_at`
	i, err := scanInterest(r.pool.QueryRowContext(ctx, q, args...))
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("UpdateInterest %d: %w", id, err)
	}
	return i, nil
}

// Delete removes an interest.
func (r *InterestRepo) Delete(ctx context.Context, id int64) error {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM interests WHERE id = ?1`
	args := []any{id}
	q, args = addUIDFilter(q, args, uid)
	_, err := r.pool.ExecContext(ctx, q, args...)
	return err
}

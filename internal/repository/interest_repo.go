package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InterestRepo accesses the interests table.
type InterestRepo struct {
	pool *pgxpool.Pool
}

// NewInterestRepo creates an InterestRepo.
func NewInterestRepo(pool *pgxpool.Pool) *InterestRepo {
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
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, created_at, updated_at FROM interests ORDER BY name`)
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
	i, err := scanInterest(r.pool.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM interests WHERE id = $1`, id))
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
	i, err := scanInterest(r.pool.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM interests WHERE name = $1`, name))
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
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM interests WHERE name=$1 AND id!=$2`, name, excludeID,
	).Scan(&n)
	return n > 0, err
}

// Create inserts a new interest.
func (r *InterestRepo) Create(ctx context.Context, name string) (*model.Interest, error) {
	i, err := scanInterest(r.pool.QueryRow(ctx,
		`INSERT INTO interests (name) VALUES ($1)
		 RETURNING id, name, created_at, updated_at`, name))
	if err != nil {
		return nil, fmt.Errorf("CreateInterest: %w", err)
	}
	return i, nil
}

// Update modifies an interest name.
func (r *InterestRepo) Update(ctx context.Context, id int64, name string) (*model.Interest, error) {
	i, err := scanInterest(r.pool.QueryRow(ctx,
		`UPDATE interests SET name=$1, updated_at=NOW() WHERE id=$2
		 RETURNING id, name, created_at, updated_at`, name, id))
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
	_, err := r.pool.Exec(ctx, `DELETE FROM interests WHERE id = $1`, id)
	return err
}

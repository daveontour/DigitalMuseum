package repository

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SensitiveRepo accesses the sensitive_data table.
type SensitiveRepo struct {
	pool *pgxpool.Pool
}

// NewSensitiveRepo creates a SensitiveRepo.
func NewSensitiveRepo(pool *pgxpool.Pool) *SensitiveRepo {
	return &SensitiveRepo{pool: pool}
}

// Count returns the number of sensitive_data rows.
func (r *SensitiveRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sensitive_data`).Scan(&n)
	return n, err
}

// KeyCount returns the number of sensitive_keyring seats.
func (r *SensitiveRepo) KeyCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sensitive_keyring`).Scan(&n)
	return n, err
}

// GetAll returns all sensitive_data rows (encrypted details).
func (r *SensitiveRepo) GetAll(ctx context.Context) ([]model.SensitiveData, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, description, details, is_private, is_sensitive, created_at, updated_at
		 FROM sensitive_data ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("GetAll sensitive_data: %w", err)
	}
	defer rows.Close()
	return scanSensitiveRows(rows)
}

// GetByID returns a single sensitive_data row by primary key.
func (r *SensitiveRepo) GetByID(ctx context.Context, id int64) (*model.SensitiveData, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, description, details, is_private, is_sensitive, created_at, updated_at
		 FROM sensitive_data WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("GetByID sensitive_data %d: %w", id, err)
	}
	defer rows.Close()
	items, err := scanSensitiveRows(rows)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &items[0], nil
}

// Create inserts a new sensitive_data row.
func (r *SensitiveRepo) Create(ctx context.Context, description, encryptedDetails string, isPrivate, isSensitive bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sensitive_data (description, details, is_private, is_sensitive)
		 VALUES ($1, $2, $3, $4)`,
		description, encryptedDetails, isPrivate, isSensitive)
	return err
}

// Update modifies an existing sensitive_data row.
func (r *SensitiveRepo) Update(ctx context.Context, id int64, description, encryptedDetails string, isPrivate, isSensitive bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sensitive_data
		 SET description=$1, details=$2, is_private=$3, is_sensitive=$4, updated_at=NOW()
		 WHERE id=$5`,
		description, encryptedDetails, isPrivate, isSensitive, id)
	return err
}

// Delete removes a sensitive_data row.
func (r *SensitiveRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sensitive_data WHERE id = $1`, id)
	return err
}

func scanSensitiveRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]model.SensitiveData, error) {
	var out []model.SensitiveData
	for rows.Next() {
		var s model.SensitiveData
		if err := rows.Scan(
			&s.ID, &s.Description, &s.Details, &s.IsPrivate, &s.IsSensitive,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

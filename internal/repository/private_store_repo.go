package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// privateStoreRow is the raw database row — value stays encrypted at this layer.
type privateStoreRow struct {
	ID             int64
	Key            string
	EncryptedValue []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PrivateStoreRepo performs raw CRUD on the private_store table.
// Encryption and decryption are handled by the service layer.
type PrivateStoreRepo struct {
	pool *pgxpool.Pool
}

// NewPrivateStoreRepo creates a PrivateStoreRepo.
func NewPrivateStoreRepo(pool *pgxpool.Pool) *PrivateStoreRepo {
	return &PrivateStoreRepo{pool: pool}
}

// GetAll returns all rows ordered by key.
func (r *PrivateStoreRepo) GetAll(ctx context.Context) ([]privateStoreRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, key, encrypted_value, created_at, updated_at FROM private_store ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("query private_store: %w", err)
	}
	defer rows.Close()

	var out []privateStoreRow
	for rows.Next() {
		var row privateStoreRow
		if err := rows.Scan(&row.ID, &row.Key, &row.EncryptedValue, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByKey returns a single row by key, or nil if not found.
func (r *PrivateStoreRepo) GetByKey(ctx context.Context, key string) (*privateStoreRow, error) {
	var row privateStoreRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, key, encrypted_value, created_at, updated_at FROM private_store WHERE key = $1`, key,
	).Scan(&row.ID, &row.Key, &row.EncryptedValue, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query private_store by key: %w", err)
	}
	return &row, nil
}

// Create inserts a new key-value pair. Returns an error if the key already exists.
func (r *PrivateStoreRepo) Create(ctx context.Context, key string, encValue []byte) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO private_store (key, encrypted_value) VALUES ($1, $2) RETURNING id`,
		key, encValue,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert private_store: %w", err)
	}
	return id, nil
}

// Upsert inserts or updates the encrypted value for key.
func (r *PrivateStoreRepo) Upsert(ctx context.Context, key string, encValue []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO private_store (key, encrypted_value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET encrypted_value = EXCLUDED.encrypted_value, updated_at = NOW()`,
		key, encValue)
	if err != nil {
		return fmt.Errorf("upsert private_store: %w", err)
	}
	return nil
}

// Update replaces the encrypted value for an existing key.
func (r *PrivateStoreRepo) Update(ctx context.Context, key string, encValue []byte) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE private_store SET encrypted_value = $1, updated_at = NOW() WHERE key = $2`,
		encValue, key)
	if err != nil {
		return fmt.Errorf("update private_store: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %q not found", key)
	}
	return nil
}

// Delete removes a row by key.
func (r *PrivateStoreRepo) Delete(ctx context.Context, key string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM private_store WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete from private_store: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %q not found", key)
	}
	return nil
}

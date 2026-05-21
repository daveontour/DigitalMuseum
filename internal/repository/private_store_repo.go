package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// privateStoreRow is the raw database row — value stays encrypted at this layer.
type privateStoreRow struct {
	ID             int64
	Key            string
	EncryptedValue []byte
	CreatedAt      sqlutil.DBTime
	UpdatedAt      sqlutil.DBTime
}

// PrivateStoreRepo performs raw CRUD on the private_store table.
// Encryption and decryption are handled by the service layer.
type PrivateStoreRepo struct {
	pool *sql.DB
}

// NewPrivateStoreRepo creates a PrivateStoreRepo.
func NewPrivateStoreRepo(pool *sql.DB) *PrivateStoreRepo {
	return &PrivateStoreRepo{pool: pool}
}

// GetAll returns all rows ordered by key.
func (r *PrivateStoreRepo) GetAll(ctx context.Context) ([]privateStoreRow, error) {
	uid := uidFromCtx(ctx)
	q := `SELECT id, key, encrypted_value, created_at, updated_at FROM private_store WHERE TRUE`
	args := []any{}
	q, args = addUIDFilter(q, args, uid)
	q += " ORDER BY key"
	rows, err := r.pool.QueryContext(ctx, q, args...)
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
	uid := uidFromCtx(ctx)
	q := `SELECT id, key, encrypted_value, created_at, updated_at FROM private_store WHERE key = ?1`
	args := []any{key}
	q, args = addUIDFilter(q, args, uid)
	var row privateStoreRow
	err := r.pool.QueryRowContext(ctx, q, args...).
		Scan(&row.ID, &row.Key, &row.EncryptedValue, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query private_store by key: %w", err)
	}
	return &row, nil
}

// Create inserts a new key-value pair. Returns an error if the key already exists.
func (r *PrivateStoreRepo) Create(ctx context.Context, key string, encValue []byte) (int64, error) {
	uid := uidFromCtx(ctx)
	var id int64
	err := r.pool.QueryRowContext(ctx,
		`INSERT INTO private_store (key, encrypted_value, user_id) VALUES (?1, ?2, ?3) RETURNING id`,
		key, encValue, uidVal(uid),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert private_store: %w", err)
	}
	return id, nil
}

// Upsert inserts or updates the encrypted value for key.
func (r *PrivateStoreRepo) Upsert(ctx context.Context, key string, encValue []byte) error {
	uid := uidFromCtx(ctx)
	_, err := r.pool.ExecContext(ctx, `
		INSERT INTO private_store (key, encrypted_value, user_id) VALUES (?1, ?2, ?3)
		ON CONFLICT (key) DO UPDATE SET encrypted_value = EXCLUDED.encrypted_value, updated_at = CURRENT_TIMESTAMP`,
		key, encValue, uidVal(uid))
	if err != nil {
		return fmt.Errorf("upsert private_store: %w", err)
	}
	return nil
}

// Update replaces the encrypted value for an existing key.
func (r *PrivateStoreRepo) Update(ctx context.Context, key string, encValue []byte) error {
	uid := uidFromCtx(ctx)
	q := `UPDATE private_store SET encrypted_value = ?1, updated_at = CURRENT_TIMESTAMP WHERE key = ?2`
	args := []any{encValue, key}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update private_store: %w", err)
	}
	if rowsAffectedOrZero(tag) == 0 {
		return fmt.Errorf("key %q not found", key)
	}
	return nil
}

// Delete removes a row by key.
func (r *PrivateStoreRepo) Delete(ctx context.Context, key string) error {
	uid := uidFromCtx(ctx)
	q := `DELETE FROM private_store WHERE key = ?1`
	args := []any{key}
	q, args = addUIDFilter(q, args, uid)
	tag, err := r.pool.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("delete from private_store: %w", err)
	}
	if rowsAffectedOrZero(tag) == 0 {
		return fmt.Errorf("key %q not found", key)
	}
	return nil
}

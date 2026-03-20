package crypto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NormalizeKeyringPassword trims and lowercases passphrases used for the sensitive keyring
// (owner master and visitor seats) so storage and unlock are case-insensitive.
func NormalizeKeyringPassword(password string) string {
	return strings.ToLower(strings.TrimSpace(password))
}

// DeriveUserKey returns a hex-encoded 32-byte key derived from password+pepper via Argon2id.
// Used as the pgp_sym_encrypt passphrase when wrapping the keyring DEK.
// The password is normalised with [NormalizeKeyringPassword] first.
func DeriveUserKey(password, pepper string) string {
	return hex.EncodeToString(DeriveKey(NormalizeKeyringPassword(password), pepper))
}

// InitSensitiveKeyring generates two fresh random DEKs, truncates sensitive_keyring
// (CASCADE clears dependent rows, e.g. visitor_key_hints), and inserts one master seat.
// The shared DEK (encrypted_dek) is accessible by any
// valid seat password. The master private DEK (encrypted_master_dek) is encrypted
// solely under masterPassword and is used for the private_store table.
func InitSensitiveKeyring(ctx context.Context, pool *pgxpool.Pool, masterPassword, pepper string) error {
	masterPassword = NormalizeKeyringPassword(masterPassword)
	if masterPassword == "" {
		return fmt.Errorf("master password required")
	}

	// Shared DEK — wrapped under master password (other seats added later via AddSensitiveKeyringSeat).
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return fmt.Errorf("generate DEK: %w", err)
	}
	dekHex := hex.EncodeToString(dek)

	// Master private DEK — exclusively for the master; never shared with other seats.
	masterDek := make([]byte, 32)
	if _, err := rand.Read(masterDek); err != nil {
		return fmt.Errorf("generate master private DEK: %w", err)
	}
	masterDekHex := hex.EncodeToString(masterDek)

	userKey := DeriveUserKey(masterPassword, pepper)

	if _, err := pool.Exec(ctx, `TRUNCATE TABLE sensitive_keyring CASCADE`); err != nil {
		return fmt.Errorf("truncate sensitive_keyring: %w", err)
	}

	var encDek []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, dekHex, userKey).Scan(&encDek); err != nil {
		return fmt.Errorf("encrypt DEK: %w", err)
	}
	var encMasterDek []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, masterDekHex, userKey).Scan(&encMasterDek); err != nil {
		return fmt.Errorf("encrypt master private DEK: %w", err)
	}

	_, err := pool.Exec(ctx,
		`INSERT INTO sensitive_keyring (encrypted_dek, encrypted_master_dek, is_master) VALUES ($1, $2, TRUE)`,
		encDek, encMasterDek)
	return err
}

// GetMasterPrivateDEK returns the hex-encoded master private DEK by decrypting
// encrypted_master_dek from the is_master=TRUE keyring row using masterPassword.
// Returns "" if the password is wrong or the column has not been populated.
func GetMasterPrivateDEK(ctx context.Context, pool *pgxpool.Pool, masterPassword, pepper string) (string, error) {
	if masterPassword == "" {
		return "", nil
	}
	userKey := DeriveUserKey(masterPassword, pepper)
	var encMasterDek []byte
	err := pool.QueryRow(ctx,
		`SELECT encrypted_master_dek FROM sensitive_keyring WHERE is_master = TRUE LIMIT 1`,
	).Scan(&encMasterDek)
	if err != nil {
		return "", fmt.Errorf("query master private DEK: %w", err)
	}
	if len(encMasterDek) == 0 {
		return "", nil
	}
	var dek string
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt($1, $2)`, encMasterDek, userKey).Scan(&dek); err != nil {
		return "", nil // wrong password — return empty, not an error
	}
	return dek, nil
}

// EncryptPrivateValue encrypts plaintext using the master private DEK.
// Returns pgcrypto-encrypted bytes for storage in private_store.encrypted_value.
func EncryptPrivateValue(ctx context.Context, pool *pgxpool.Pool, masterPassword, plaintext, pepper string) ([]byte, error) {
	dek, err := GetMasterPrivateDEK(ctx, pool, masterPassword, pepper)
	if err != nil {
		return nil, fmt.Errorf("get master private DEK: %w", err)
	}
	if dek == "" {
		return nil, fmt.Errorf("invalid master password or master private DEK not initialised")
	}
	var enc []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, plaintext, dek).Scan(&enc); err != nil {
		return nil, fmt.Errorf("pgp_sym_encrypt: %w", err)
	}
	return enc, nil
}

// DecryptPrivateValue decrypts a private_store.encrypted_value using the master private DEK.
// Returns "" (no error) if masterPassword is wrong or the DEK is not initialised.
func DecryptPrivateValue(ctx context.Context, pool *pgxpool.Pool, masterPassword string, encValue []byte, pepper string) (string, error) {
	dek, err := GetMasterPrivateDEK(ctx, pool, masterPassword, pepper)
	if err != nil {
		return "", fmt.Errorf("get master private DEK: %w", err)
	}
	if dek == "" {
		return "", nil
	}
	var plain string
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt($1, $2)`, encValue, dek).Scan(&plain); err != nil {
		return "", fmt.Errorf("pgp_sym_decrypt: %w", err)
	}
	return plain, nil
}

// GetSensitiveDEK scans all sensitive_keyring rows and returns the hex DEK that decrypts
// successfully with userPassword. Returns "" if no matching seat is found.
func GetSensitiveDEK(ctx context.Context, pool *pgxpool.Pool, userPassword, pepper string) (string, error) {
	if userPassword == "" {
		return "", nil
	}
	userKey := DeriveUserKey(userPassword, pepper)
	rows, err := pool.Query(ctx, `SELECT encrypted_dek FROM sensitive_keyring`)
	if err != nil {
		return "", fmt.Errorf("query sensitive_keyring: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var encDek []byte
		if err := rows.Scan(&encDek); err != nil {
			return "", err
		}
		var dek string
		if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt($1, $2)`, encDek, userKey).Scan(&dek); err == nil {
			return dek, nil
		}
	}
	return "", rows.Err()
}

// CheckSensitiveMasterPassword returns true if password decrypts any is_master=TRUE keyring row.
func CheckSensitiveMasterPassword(ctx context.Context, pool *pgxpool.Pool, password, pepper string) (bool, error) {
	if password == "" {
		return false, nil
	}
	userKey := DeriveUserKey(password, pepper)
	rows, err := pool.Query(ctx, `SELECT encrypted_dek FROM sensitive_keyring WHERE is_master = TRUE`)
	if err != nil {
		return false, fmt.Errorf("query sensitive_keyring: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var encDek []byte
		if err := rows.Scan(&encDek); err != nil {
			return false, err
		}
		var dek string
		if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt($1, $2)`, encDek, userKey).Scan(&dek); err == nil {
			return true, nil
		}
	}
	return false, rows.Err()
}

// CheckSensitiveVisitorSeatPassword reports whether password unlocks a non-master keyring seat.
// It returns false if the password is empty, if it is the owner master key (use CheckSensitiveMasterPassword),
// or if no seat matches. Does not return true for the master row.
func CheckSensitiveVisitorSeatPassword(ctx context.Context, pool *pgxpool.Pool, password, pepper string) (bool, error) {
	if password == "" {
		return false, nil
	}
	isMaster, err := CheckSensitiveMasterPassword(ctx, pool, password, pepper)
	if err != nil {
		return false, err
	}
	if isMaster {
		return false, nil
	}
	dek, err := GetSensitiveDEK(ctx, pool, password, pepper)
	if err != nil {
		return false, err
	}
	return dek != "", nil
}

// AddSensitiveKeyringSeatTx adds a new non-master keyring seat inside an open transaction.
// pool is used read-only for DEK recovery; writes go through tx.
func AddSensitiveKeyringSeatTx(ctx context.Context, tx pgx.Tx, pool *pgxpool.Pool, newUserPassword, masterPassword, pepper string) (int64, error) {
	newUserPassword = NormalizeKeyringPassword(newUserPassword)
	if newUserPassword == "" {
		return 0, fmt.Errorf("new user password required")
	}
	dek, err := GetSensitiveDEK(ctx, pool, masterPassword, pepper)
	if err != nil {
		return 0, fmt.Errorf("get DEK: %w", err)
	}
	if dek == "" {
		return 0, fmt.Errorf("invalid master password or no keyring initialised")
	}
	newUserKey := DeriveUserKey(newUserPassword, pepper)
	var encDek []byte
	if err := tx.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, dek, newUserKey).Scan(&encDek); err != nil {
		return 0, fmt.Errorf("encrypt DEK for new user: %w", err)
	}
	var id int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO sensitive_keyring (encrypted_dek, is_master) VALUES ($1, FALSE) RETURNING id`,
		encDek).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// AddSensitiveKeyringSeat adds a new non-master keyring seat for newUserPassword.
// Requires masterPassword to recover the existing DEK first. Returns the new sensitive_keyring.id.
func AddSensitiveKeyringSeat(ctx context.Context, pool *pgxpool.Pool, newUserPassword, masterPassword, pepper string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	id, err := AddSensitiveKeyringSeatTx(ctx, tx, pool, newUserPassword, masterPassword, pepper)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// DeleteSensitiveKeyringSeat removes the keyring seat for userPassword.
// Requires masterPassword for authorisation. Refuses to remove master seats.
func DeleteSensitiveKeyringSeat(ctx context.Context, pool *pgxpool.Pool, userPassword, masterPassword, pepper string) error {
	ok, err := CheckSensitiveMasterPassword(ctx, pool, masterPassword, pepper)
	if err != nil {
		return fmt.Errorf("check master password: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid master password")
	}
	userKey := DeriveUserKey(userPassword, pepper)
	rows, err := pool.Query(ctx, `SELECT id, encrypted_dek, is_master FROM sensitive_keyring`)
	if err != nil {
		return fmt.Errorf("query sensitive_keyring: %w", err)
	}
	defer rows.Close()
	var matchID int
	var matchMaster bool
	for rows.Next() {
		var id int
		var encDek []byte
		var isMaster bool
		if err := rows.Scan(&id, &encDek, &isMaster); err != nil {
			return err
		}
		var dek string
		if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt($1, $2)`, encDek, userKey).Scan(&dek); err == nil {
			matchID = id
			matchMaster = isMaster
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if matchID == 0 {
		return fmt.Errorf("no keyring seat found for supplied password")
	}
	if matchMaster {
		return fmt.Errorf("cannot remove master keyring seat")
	}
	_, err = pool.Exec(ctx, `DELETE FROM sensitive_keyring WHERE id = $1`, matchID)
	return err
}

// DeleteAllVisitorKeyringSeats removes every non-master keyring seat (visitor keys).
// Associated rows in visitor_key_hints are removed via ON DELETE CASCADE.
// The master seat (is_master = TRUE) is never deleted. masterPassword must decrypt the master row.
func DeleteAllVisitorKeyringSeats(ctx context.Context, pool *pgxpool.Pool, masterPassword, pepper string) (int64, error) {
	ok, err := CheckSensitiveMasterPassword(ctx, pool, masterPassword, pepper)
	if err != nil {
		return 0, fmt.Errorf("check master password: %w", err)
	}
	if !ok {
		return 0, fmt.Errorf("invalid master password")
	}
	ct, err := pool.Exec(ctx, `DELETE FROM sensitive_keyring WHERE is_master = FALSE`)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// DeleteVisitorKeyringSeatByID removes a single visitor seat by sensitive_keyring.id.
// Master seats are never removed. masterPassword must decrypt the master row.
func DeleteVisitorKeyringSeatByID(ctx context.Context, pool *pgxpool.Pool, keyringID int64, masterPassword, pepper string) error {
	ok, err := CheckSensitiveMasterPassword(ctx, pool, masterPassword, pepper)
	if err != nil {
		return fmt.Errorf("check master password: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid master password")
	}
	tag, err := pool.Exec(ctx, `DELETE FROM sensitive_keyring WHERE id = $1 AND is_master = FALSE`, keyringID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("visitor keyring seat not found")
	}
	return nil
}

// SensitiveKeyringSeatCount returns the total number of rows in sensitive_keyring.
func SensitiveKeyringSeatCount(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sensitive_keyring`).Scan(&count)
	return count, err
}

// EncryptDocumentData encrypts raw bytes using the sensitive_keyring DEK.
// Returns the pgcrypto-encrypted BYTEA for storage in reference_documents.data.
func EncryptDocumentData(ctx context.Context, pool *pgxpool.Pool, masterPassword string, data []byte, pepper string) ([]byte, error) {
	dek, err := GetSensitiveDEK(ctx, pool, masterPassword, pepper)
	if err != nil {
		return nil, fmt.Errorf("get DEK: %w", err)
	}
	if dek == "" {
		return nil, fmt.Errorf("invalid master password or no keyring initialised")
	}
	var enc []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt_bytea($1, $2)`, data, dek).Scan(&enc); err != nil {
		return nil, fmt.Errorf("pgp_sym_encrypt_bytea: %w", err)
	}
	return enc, nil
}

// DecryptDocumentData decrypts a pgcrypto-encrypted BYTEA from reference_documents.data.
// Returns nil bytes (no error) if userPassword has no matching keyring seat.
func DecryptDocumentData(ctx context.Context, pool *pgxpool.Pool, userPassword string, encData []byte, pepper string) ([]byte, error) {
	dek, err := GetSensitiveDEK(ctx, pool, userPassword, pepper)
	if err != nil {
		return nil, fmt.Errorf("get DEK: %w", err)
	}
	if dek == "" {
		return nil, nil
	}
	var plain []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt_bytea($1, $2)`, encData, dek).Scan(&plain); err != nil {
		return nil, fmt.Errorf("pgp_sym_decrypt_bytea: %w", err)
	}
	return plain, nil
}

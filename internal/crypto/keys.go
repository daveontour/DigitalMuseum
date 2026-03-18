package crypto

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DeriveUserKey returns a hex-encoded 32-byte key derived from password+pepper via Argon2id.
// Used as the pgp_sym_encrypt passphrase when wrapping the keyring DEK.
func DeriveUserKey(password, pepper string) string {
	return hex.EncodeToString(DeriveKey(password, pepper))
}

// InitSensitiveKeyring generates a fresh random DEK, truncates sensitive_keyring,
// and inserts one master seat encrypted under masterPassword.
// It does NOT truncate sensitive_data — existing records must be migrated separately.
func InitSensitiveKeyring(ctx context.Context, pool *pgxpool.Pool, masterPassword, pepper string) error {
	if masterPassword == "" {
		return fmt.Errorf("master password required")
	}
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return fmt.Errorf("generate DEK: %w", err)
	}
	dekHex := hex.EncodeToString(dek)
	userKey := DeriveUserKey(masterPassword, pepper)

	if _, err := pool.Exec(ctx, `TRUNCATE TABLE sensitive_keyring`); err != nil {
		return fmt.Errorf("truncate sensitive_keyring: %w", err)
	}
	var encDek []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, dekHex, userKey).Scan(&encDek); err != nil {
		return fmt.Errorf("encrypt DEK: %w", err)
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO sensitive_keyring (encrypted_dek, is_master) VALUES ($1, TRUE)`, encDek)
	return err
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

// AddSensitiveKeyringSeat adds a new non-master keyring seat for newUserPassword.
// Requires masterPassword to recover the existing DEK first.
func AddSensitiveKeyringSeat(ctx context.Context, pool *pgxpool.Pool, newUserPassword, masterPassword, pepper string) error {
	if newUserPassword == "" {
		return fmt.Errorf("new user password required")
	}
	dek, err := GetSensitiveDEK(ctx, pool, masterPassword, pepper)
	if err != nil {
		return fmt.Errorf("get DEK: %w", err)
	}
	if dek == "" {
		return fmt.Errorf("invalid master password or no keyring initialised")
	}
	newUserKey := DeriveUserKey(newUserPassword, pepper)
	var encDek []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, dek, newUserKey).Scan(&encDek); err != nil {
		return fmt.Errorf("encrypt DEK for new user: %w", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO sensitive_keyring (encrypted_dek, is_master) VALUES ($1, FALSE)`, encDek)
	return err
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

// SensitiveKeyringSeatCount returns the total number of rows in sensitive_keyring.
func SensitiveKeyringSeatCount(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sensitive_keyring`).Scan(&count)
	return count, err
}

// EncryptSensitiveRecord encrypts details with the keyring DEK and returns a base64 string
// suitable for storage in sensitive_data.details.
func EncryptSensitiveRecord(ctx context.Context, pool *pgxpool.Pool, masterPassword, details, pepper string) (string, error) {
	dek, err := GetSensitiveDEK(ctx, pool, masterPassword, pepper)
	if err != nil {
		return "", fmt.Errorf("get DEK: %w", err)
	}
	if dek == "" {
		return "", fmt.Errorf("invalid master password or no keyring initialised")
	}
	var enc []byte
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_encrypt($1, $2)`, details, dek).Scan(&enc); err != nil {
		return "", fmt.Errorf("pgp_sym_encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

// DecryptSensitiveRecord decrypts a base64-encoded sensitive_data.details field using
// the keyring DEK. Returns "" if userPassword has no matching seat (caller handles redaction).
func DecryptSensitiveRecord(ctx context.Context, pool *pgxpool.Pool, userPassword, encDetails, pepper string) (string, error) {
	dek, err := GetSensitiveDEK(ctx, pool, userPassword, pepper)
	if err != nil {
		return "", fmt.Errorf("get DEK: %w", err)
	}
	if dek == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(encDetails)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	var plain string
	if err := pool.QueryRow(ctx, `SELECT pgp_sym_decrypt($1::bytea, $2)`, data, dek).Scan(&plain); err != nil {
		return "", fmt.Errorf("pgp_sym_decrypt: %w", err)
	}
	return plain, nil
}

// MigrateSensitiveRecords re-encrypts records still in the old RSA/hybrid format.
// Requires oldMasterPassword to recover the legacy RSA private key from trusted_keys,
// and newMasterPassword to encrypt with the new pgcrypto DEK.
// Records that already decrypt with the new system are skipped.
// Returns the count of records migrated.
func MigrateSensitiveRecords(ctx context.Context, pool *pgxpool.Pool, oldMasterPassword, newMasterPassword, pepper string) (int, error) {
	dek, err := GetSensitiveDEK(ctx, pool, newMasterPassword, pepper)
	if err != nil {
		return 0, fmt.Errorf("get new DEK: %w", err)
	}
	if dek == "" {
		return 0, fmt.Errorf("new keyring not initialised — call init-keyring first")
	}

	privPEM, err := legacyGetPrivateKey(ctx, pool, oldMasterPassword, pepper)
	if err != nil {
		return 0, fmt.Errorf("recover old private key: %w", err)
	}
	if privPEM == "" {
		return 0, fmt.Errorf("old master private key not found — check old password")
	}

	rows, err := pool.Query(ctx, `SELECT id, details FROM sensitive_data WHERE details IS NOT NULL AND details != ''`)
	if err != nil {
		return 0, fmt.Errorf("query sensitive_data: %w", err)
	}
	defer rows.Close()

	type record struct {
		id      int64
		details string
	}
	var records []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.details); err != nil {
			return 0, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	migrated := 0
	for _, r := range records {
		// Skip records already in new format.
		if _, err := DecryptSensitiveRecord(ctx, pool, newMasterPassword, r.details, pepper); err == nil {
			continue
		}
		plain, err := legacyDecryptRecord(privPEM, r.details)
		if err != nil {
			continue // skip records that cannot be decrypted
		}
		newEnc, err := EncryptSensitiveRecord(ctx, pool, newMasterPassword, plain, pepper)
		if err != nil {
			return migrated, fmt.Errorf("re-encrypt record %d: %w", r.id, err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE sensitive_data SET details = $1, updated_at = NOW() WHERE id = $2`,
			newEnc, r.id,
		); err != nil {
			return migrated, fmt.Errorf("update record %d: %w", r.id, err)
		}
		migrated++
	}
	return migrated, nil
}

// legacyGetPrivateKey replicates the old GetPrivateKey using EncodedPassword + Decrypt
// against trusted_keys. Used only by MigrateSensitiveRecords.
func legacyGetPrivateKey(ctx context.Context, pool *pgxpool.Pool, password, pepper string) (string, error) {
	rows, err := pool.Query(ctx, `SELECT key FROM trusted_keys`)
	if err != nil {
		return "", fmt.Errorf("query trusted_keys: %w", err)
	}
	defer rows.Close()
	encoded := EncodedPassword(password, pepper)
	for rows.Next() {
		var encKey string
		if err := rows.Scan(&encKey); err != nil {
			return "", err
		}
		if plain, err := Decrypt(encKey, encoded, pepper); err == nil {
			return plain, nil
		}
	}
	return "", rows.Err()
}

// legacyDecryptRecord decrypts a base64 RSA-hybrid record. Used only by MigrateSensitiveRecords.
func legacyDecryptRecord(privateKeyPEM, encryptedB64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	plain, err := DecryptWithPrivateKeyHybrid([]byte(privateKeyPEM), data)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// EnsureKeyringSeat checks whether userPassword already has a valid keyring seat
// and adds one if it does not. masterPassword is required to recover the existing DEK;
// if it is empty or invalid the function returns an error.
// Returns (true, nil) if the seat already existed, (false, nil) if it was just added.
func EnsureKeyringSeat(ctx context.Context, pool *pgxpool.Pool, userPassword, masterPassword, pepper string) (alreadyExisted bool, err error) {
	if userPassword == "" {
		return false, fmt.Errorf("user password is required")
	}
	// Check whether the seat already exists.
	dek, err := GetSensitiveDEK(ctx, pool, userPassword, pepper)
	if err != nil {
		return false, fmt.Errorf("check keyring: %w", err)
	}
	if dek != "" {
		return true, nil
	}
	// Seat missing — add it.
	if err := AddSensitiveKeyringSeat(ctx, pool, userPassword, masterPassword, pepper); err != nil {
		return false, fmt.Errorf("add keyring seat: %w", err)
	}
	return false, nil
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

package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
)

// ErrEncryptionDisabled is kept for callers that branch on a disabled crypto build.
// This file implements keyring + private-store crypto for SQLite / single-user deployments.
var ErrEncryptionDisabled = errors.New("encryption and sensitive keyring are not available in this build")

const dekLen = 32

func aesGCMSeal(key, plaintext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, out...), nil
}

func aesGCMOpen(key, sealed []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(sealed) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, sealed[:ns], sealed[ns:], nil)
}

func sealDEKWithPassword(archiveDEK []byte, password, pepper string) ([]byte, error) {
	key := DeriveKey(NormalizeKeyringPassword(password), pepper)
	return aesGCMSeal(key, archiveDEK)
}

func openDEKWithPassword(blob []byte, password, pepper string) ([]byte, error) {
	key := DeriveKey(NormalizeKeyringPassword(password), pepper)
	return aesGCMOpen(key, blob)
}

func randomDEK() ([]byte, error) {
	b := make([]byte, dekLen)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

func deleteKeyringRowsForUser(ctx context.Context, exec sqlExecuter, uid int64) error {
	if uid > 0 {
		_, err := exec.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE user_id = ?1`, uid)
		return err
	}
	_, err := exec.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE user_id IS NULL`)
	return err
}

// sqlExecuter is *sql.DB or *sql.Tx.
type sqlExecuter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func scanMasterEncryptedDEK(ctx context.Context, q sqlRowQuerier, uid int64) ([]byte, error) {
	var enc []byte
	var err error
	if uid > 0 {
		err = q.QueryRowContext(ctx,
			`SELECT encrypted_dek FROM sensitive_keyring WHERE is_master = TRUE AND user_id = ?1 LIMIT 1`,
			uid).Scan(&enc)
	} else {
		err = q.QueryRowContext(ctx,
			`SELECT encrypted_dek FROM sensitive_keyring WHERE is_master = TRUE AND user_id IS NULL LIMIT 1`,
		).Scan(&enc)
	}
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no master keyring row")
	}
	if err != nil {
		return nil, err
	}
	return enc, nil
}

type sqlRowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// decryptArchiveDEKWithMaster returns the 32-byte archive DEK using the master password row.
func decryptArchiveDEKWithMaster(ctx context.Context, q sqlRowQuerier, masterPassword, pepper string, uid int64) ([]byte, error) {
	enc, err := scanMasterEncryptedDEK(ctx, q, uid)
	if err != nil {
		return nil, err
	}
	return openDEKWithPassword(enc, masterPassword, pepper)
}

// NormalizeKeyringPassword trims and lowercases passphrases used for the sensitive keyring
// (owner master and visitor seats) so storage and unlock are case-insensitive.
func NormalizeKeyringPassword(password string) string {
	return strings.ToLower(strings.TrimSpace(password))
}

// DeriveUserKey returns a hex-encoded 32-byte key derived from password+pepper via Argon2id.
func DeriveUserKey(password, pepper string) string {
	return hex.EncodeToString(DeriveKey(NormalizeKeyringPassword(password), pepper))
}

// InitSensitiveKeyring replaces all keyring seats for this archive user with a fresh master seat.
func InitSensitiveKeyring(ctx context.Context, db *sql.DB, masterPassword, pepper string) error {
	norm := NormalizeKeyringPassword(masterPassword)
	if norm == "" {
		return fmt.Errorf("master password required")
	}
	uid := appctx.UserIDFromCtx(ctx)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteKeyringRowsForUser(ctx, tx, uid); err != nil {
		return err
	}
	k, err := randomDEK()
	if err != nil {
		return err
	}
	blob, err := sealDEKWithPassword(k, norm, pepper)
	if err != nil {
		return err
	}
	if uid > 0 {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO sensitive_keyring (encrypted_dek, encrypted_master_dek, is_master, user_id) VALUES (?1, NULL, TRUE, ?2)`,
			blob, uid)
	} else {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO sensitive_keyring (encrypted_dek, encrypted_master_dek, is_master, user_id) VALUES (?1, NULL, TRUE, NULL)`,
			blob)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetMasterPrivateDEK returns the hex-encoded archive DEK after decrypting the master row.
func GetMasterPrivateDEK(ctx context.Context, db *sql.DB, masterPassword, pepper string) (string, error) {
	uid := appctx.UserIDFromCtx(ctx)
	k, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, uid)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(k), nil
}

// GetSensitiveDEK returns the hex-encoded archive DEK using master password or a visitor seat password.
func GetSensitiveDEK(ctx context.Context, db *sql.DB, password, pepper string) (string, error) {
	k, err := unlockArchiveDEK(ctx, db, password, pepper)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(k), nil
}

func unlockArchiveDEK(ctx context.Context, db *sql.DB, password, pepper string) ([]byte, error) {
	uid := appctx.UserIDFromCtx(ctx)
	if k, err := decryptArchiveDEKWithMaster(ctx, db, password, pepper, uid); err == nil {
		return k, nil
	}
	var rows *sql.Rows
	var err error
	if uid > 0 {
		rows, err = db.QueryContext(ctx,
			`SELECT encrypted_dek FROM sensitive_keyring WHERE is_master = FALSE AND user_id = ?1`, uid)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT encrypted_dek FROM sensitive_keyring WHERE is_master = FALSE AND user_id IS NULL`)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var enc []byte
		if err := rows.Scan(&enc); err != nil {
			return nil, err
		}
		if k, err := openDEKWithPassword(enc, password, pepper); err == nil && len(k) == dekLen {
			return k, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("password does not unlock this keyring")
}

// EncryptPrivateValue encrypts plaintext with the archive DEK (master password unwraps storage key).
func EncryptPrivateValue(ctx context.Context, db *sql.DB, masterPassword, plaintext, pepper string) ([]byte, error) {
	k, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, appctx.UserIDFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	return aesGCMSeal(k, []byte(plaintext))
}

// DecryptPrivateValue decrypts a value stored with EncryptPrivateValue.
func DecryptPrivateValue(ctx context.Context, db *sql.DB, masterPassword string, encData []byte, pepper string) (string, error) {
	if len(encData) == 0 {
		return "", nil
	}
	k, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, appctx.UserIDFromCtx(ctx))
	if err != nil {
		return "", err
	}
	plain, err := aesGCMOpen(k, encData)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// CheckSensitiveMasterPassword returns true when masterPassword decrypts the master keyring row.
func CheckSensitiveMasterPassword(ctx context.Context, db *sql.DB, masterPassword, pepper string) (bool, error) {
	uid := appctx.UserIDFromCtx(ctx)
	_, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, uid)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// FindVisitorKeyringIDForPassword returns the visitor seat id when password unlocks that row.
func FindVisitorKeyringIDForPassword(ctx context.Context, db *sql.DB, password, pepper string) (keyringID int64, ok bool, err error) {
	uid := appctx.UserIDFromCtx(ctx)
	var rows *sql.Rows
	if uid > 0 {
		rows, err = db.QueryContext(ctx,
			`SELECT id, encrypted_dek FROM sensitive_keyring WHERE is_master = FALSE AND user_id = ?1`, uid)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT id, encrypted_dek FROM sensitive_keyring WHERE is_master = FALSE AND user_id IS NULL`)
	}
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var enc []byte
		if err := rows.Scan(&id, &enc); err != nil {
			return 0, false, err
		}
		if _, err := openDEKWithPassword(enc, password, pepper); err == nil {
			return id, true, nil
		}
	}
	return 0, false, rows.Err()
}

// CheckSensitiveVisitorSeatPassword returns true when password unlocks a non-master seat only.
func CheckSensitiveVisitorSeatPassword(ctx context.Context, db *sql.DB, password, pepper string) (bool, error) {
	if ok, _ := CheckSensitiveMasterPassword(ctx, db, password, pepper); ok {
		return false, nil
	}
	_, ok, err := FindVisitorKeyringIDForPassword(ctx, db, password, pepper)
	return ok, err
}

// AddSensitiveKeyringSeatTx inserts a visitor seat wrapping the same archive DEK with userPassword.
func AddSensitiveKeyringSeatTx(ctx context.Context, tx *sql.Tx, _ *sql.DB, userPassword, masterPassword, pepper string) (int64, error) {
	uid := appctx.UserIDFromCtx(ctx)
	k, err := decryptArchiveDEKWithMaster(ctx, tx, masterPassword, pepper, uid)
	if err != nil {
		return 0, fmt.Errorf("invalid master password or keyring not initialised: %w", err)
	}
	up := NormalizeKeyringPassword(userPassword)
	if up == "" {
		return 0, fmt.Errorf("visitor password is required")
	}
	blob, err := sealDEKWithPassword(k, up, pepper)
	if err != nil {
		return 0, err
	}
	var id int64
	if uid > 0 {
		err = tx.QueryRowContext(ctx,
			`INSERT INTO sensitive_keyring (encrypted_dek, encrypted_master_dek, is_master, user_id) VALUES (?1, NULL, FALSE, ?2) RETURNING id`,
			blob, uid).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx,
			`INSERT INTO sensitive_keyring (encrypted_dek, encrypted_master_dek, is_master, user_id) VALUES (?1, NULL, FALSE, NULL) RETURNING id`,
			blob).Scan(&id)
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// AddSensitiveKeyringSeat inserts a visitor seat in its own transaction.
func AddSensitiveKeyringSeat(ctx context.Context, db *sql.DB, userPassword, masterPassword, pepper string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	id, err := AddSensitiveKeyringSeatTx(ctx, tx, db, userPassword, masterPassword, pepper)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// DeleteSensitiveKeyringSeat removes a visitor seat matching userPassword; masterPassword authorises.
func DeleteSensitiveKeyringSeat(ctx context.Context, db *sql.DB, userPassword, masterPassword, pepper string) error {
	uid := appctx.UserIDFromCtx(ctx)
	if _, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, uid); err != nil {
		return fmt.Errorf("invalid master password: %w", err)
	}
	id, ok, err := FindVisitorKeyringIDForPassword(ctx, db, userPassword, pepper)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("visitor password does not match any seat")
	}
	if uid > 0 {
		_, err = db.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE id = ?1 AND is_master = FALSE AND user_id = ?2`, id, uid)
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE id = ?1 AND is_master = FALSE AND user_id IS NULL`, id)
	}
	return err
}

// DeleteAllVisitorKeyringSeats removes every non-master seat after master password verification.
func DeleteAllVisitorKeyringSeats(ctx context.Context, db *sql.DB, masterPassword, pepper string) (int64, error) {
	uid := appctx.UserIDFromCtx(ctx)
	if _, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, uid); err != nil {
		return 0, fmt.Errorf("invalid master password: %w", err)
	}
	var res sql.Result
	var err error
	if uid > 0 {
		res, err = db.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE is_master = FALSE AND user_id = ?1`, uid)
	} else {
		res, err = db.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE is_master = FALSE AND user_id IS NULL`)
	}
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeleteVisitorKeyringSeatByID removes a visitor seat by id after master verification.
func DeleteVisitorKeyringSeatByID(ctx context.Context, db *sql.DB, keyringID int64, masterPassword, pepper string) error {
	uid := appctx.UserIDFromCtx(ctx)
	if _, err := decryptArchiveDEKWithMaster(ctx, db, masterPassword, pepper, uid); err != nil {
		return fmt.Errorf("invalid master password: %w", err)
	}
	var isMaster bool
	var err error
	if uid > 0 {
		err = db.QueryRowContext(ctx,
			`SELECT is_master FROM sensitive_keyring WHERE id = ?1 AND user_id = ?2`, keyringID, uid).Scan(&isMaster)
	} else {
		err = db.QueryRowContext(ctx,
			`SELECT is_master FROM sensitive_keyring WHERE id = ?1 AND user_id IS NULL`, keyringID).Scan(&isMaster)
	}
	if err == sql.ErrNoRows {
		return fmt.Errorf("keyring seat not found")
	}
	if err != nil {
		return err
	}
	if isMaster {
		return fmt.Errorf("cannot remove master seat")
	}
	if uid > 0 {
		_, err = db.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE id = ?1 AND user_id = ?2`, keyringID, uid)
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM sensitive_keyring WHERE id = ?1 AND user_id IS NULL`, keyringID)
	}
	return err
}

// SensitiveKeyringSeatCount returns the number of keyring rows for this archive user.
func SensitiveKeyringSeatCount(ctx context.Context, db *sql.DB) (int, error) {
	uid := appctx.UserIDFromCtx(ctx)
	var n int
	var err error
	if uid > 0 {
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensitive_keyring WHERE user_id = ?1`, uid).Scan(&n)
	} else {
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensitive_keyring WHERE user_id IS NULL`).Scan(&n)
	}
	return n, err
}

// EncryptDocumentData encrypts document bytes with the archive DEK derived from password (master or visitor).
func EncryptDocumentData(ctx context.Context, db *sql.DB, password string, data []byte, pepper string) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	k, err := unlockArchiveDEK(ctx, db, password, pepper)
	if err != nil {
		return nil, err
	}
	return aesGCMSeal(k, data)
}

// DecryptDocumentData decrypts document bytes stored with EncryptDocumentData.
func DecryptDocumentData(ctx context.Context, db *sql.DB, password string, encData []byte, pepper string) ([]byte, error) {
	if len(encData) == 0 {
		return nil, nil
	}
	k, err := unlockArchiveDEK(ctx, db, password, pepper)
	if err != nil {
		return nil, err
	}
	return aesGCMOpen(k, encData)
}

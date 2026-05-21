package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

const redacted = "*****************"

// SensitiveService handles sensitive-data CRUD and keyring management.
// Sensitive records are stored as reference_documents with is_sensitive=TRUE.
type SensitiveService struct {
	docRepo *repository.DocumentRepo
	pool    *sql.DB
	pepper  string
}

// NewSensitiveService creates a SensitiveService backed by DocumentRepo.
// pepper is ATTACHMENT_ALLOWED_TYPES from config (used for key derivation).
func NewSensitiveService(docRepo *repository.DocumentRepo, pool *sql.DB, pepper string) *SensitiveService {
	return &SensitiveService{docRepo: docRepo, pool: pool, pepper: pepper}
}

// uidFilter appends a user_id condition to q based on the authenticated user in ctx.
func (s *SensitiveService) uidFilter(ctx context.Context, q string, args []any) (string, []any) {
	uid := appctx.UserIDFromCtx(ctx)
	if uid > 0 {
		args = append(args, uid)
		return q + fmt.Sprintf(" AND user_id = ?%d", len(args)), args
	}
	return q + " AND user_id IS NULL", args
}

// Count returns the total number of sensitive records in reference_documents for this user.
func (s *SensitiveService) Count(ctx context.Context) (int64, error) {
	q, args := s.uidFilter(ctx, `SELECT COUNT(*) FROM reference_documents WHERE is_sensitive = TRUE`, nil)
	var n int64
	err := s.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// KeyCount returns the total number of sensitive_keyring seats for this user.
// If the table is missing (very old DB), the count is treated as zero.
func (s *SensitiveService) KeyCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM sensitive_keyring`).Scan(&n)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return 0, nil
	}
	return n, err
}

// VerifyMasterPassword returns true if masterPassword decrypts the master (is_master) keyring row.
func (s *SensitiveService) VerifyMasterPassword(ctx context.Context, masterPassword string) (bool, error) {
	return appcrypto.CheckSensitiveMasterPassword(ctx, s.pool, masterPassword, s.pepper)
}

// InitKeyring initialises the master sensitive_keyring seat for the current user
// using masterPassword as both the unlock key and the key-derivation input.
// Any existing keyring rows for this user are replaced.
func (s *SensitiveService) InitKeyring(ctx context.Context, masterPassword string) error {
	return appcrypto.InitSensitiveKeyring(ctx, s.pool, masterPassword, s.pepper)
}

// SeedNewOwnerDefaults enables all LLM tools and background jobs for a new archive owner.
func (s *SensitiveService) SeedNewOwnerDefaults(ctx context.Context, masterPassword string) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("sensitive service not configured")
	}
	return SeedNewOwnerArchiveDefaults(ctx, s.pool, masterPassword, s.pepper)
}

// VerifyVisitorKeyringPassword returns true if password unlocks a non-master keyring seat only
// (not the owner master password).
func (s *SensitiveService) VerifyVisitorKeyringPassword(ctx context.Context, password string) (bool, error) {
	return appcrypto.CheckSensitiveVisitorSeatPassword(ctx, s.pool, password, s.pepper)
}

// ListAll returns all sensitive records. If password is empty details are redacted.
func (s *SensitiveService) ListAll(ctx context.Context, password string) ([]model.SensitiveDataResponse, error) {
	docs, err := s.docRepo.ListSensitive(ctx)
	if err != nil {
		return nil, err
	}
	return s.toResponses(ctx, docs, password), nil
}

// GetByID returns a single sensitive record, decrypting if password is valid.
func (s *SensitiveService) GetByID(ctx context.Context, id int64, password string) (*model.SensitiveDataResponse, error) {
	doc, err := s.docRepo.GetSensitiveByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}
	responses := s.toResponses(ctx, []*model.ReferenceDocument{doc}, password)
	return &responses[0], nil
}

// Create encrypts details with the keyring DEK and stores it as a sensitive reference_document.
func (s *SensitiveService) Create(ctx context.Context, masterPassword, description, details string, isPrivate, isSensitive, includeInSystemPrompt bool) error {
	data := []byte(details)
	enc, err := appcrypto.EncryptDocumentData(ctx, s.pool, masterPassword, data, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt record: %w", err)
	}
	title := description
	_, err = s.docRepo.Create(ctx,
		description, "text/plain", int64(len(data)), enc,
		&title, nil, nil, nil, nil, nil,
		false, includeInSystemPrompt, isPrivate, isSensitive, true,
	)
	return err
}

// Update re-encrypts details and updates the record.
func (s *SensitiveService) Update(ctx context.Context, id int64, masterPassword, description, details string, isPrivate, isSensitive, includeInSystemPrompt bool) error {
	data := []byte(details)
	enc, err := appcrypto.EncryptDocumentData(ctx, s.pool, masterPassword, data, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt record: %w", err)
	}
	title := description
	inc := includeInSystemPrompt
	if _, err := s.docRepo.Update(ctx, id, &title, nil, nil, nil, nil, nil, nil, &inc); err != nil {
		return err
	}
	return s.docRepo.UpdateData(ctx, id, enc, true)
}

// Delete removes a sensitive record. Requires a valid master password.
func (s *SensitiveService) Delete(ctx context.Context, id int64, masterPassword string) error {
	ok, err := appcrypto.CheckSensitiveMasterPassword(ctx, s.pool, masterPassword, s.pepper)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("invalid master password")
	}
	return s.docRepo.Delete(ctx, id)
}

// GenerateKeyring initialises a fresh pgcrypto keyring for the master password.
func (s *SensitiveService) GenerateKeyring(ctx context.Context, masterPassword string) error {
	return appcrypto.InitSensitiveKeyring(ctx, s.pool, masterPassword, s.pepper)
}

const maxVisitorKeyHintLen = 2000

// VisitorKeyFeatureFlags selects which archive areas a visitor key may use (owner always has full access).
type VisitorKeyFeatureFlags struct {
	CanMessagesChat     bool
	CanEmails           bool
	CanContacts         bool
	CanRelationships    bool // DB column can_relationship_sensitive
	CanSensitivePrivate bool // DB column can_sensitive_private
	LLMAllowOwnerKeys   bool
	LLMAllowServerKeys  bool
}

// AddUser adds a new keyring seat for userPassword and stores a plain-text hint for the unlock dialog.
func (s *SensitiveService) AddUser(ctx context.Context, userPassword, masterPassword, hint string, flags VisitorKeyFeatureFlags) error {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return fmt.Errorf("visitor key hint is required")
	}
	if len(hint) > maxVisitorKeyHintLen {
		return fmt.Errorf("hint exceeds %d characters", maxVisitorKeyHintLen)
	}
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keyringID, err := appcrypto.AddSensitiveKeyringSeatTx(ctx, tx, s.pool, userPassword, masterPassword, s.pepper)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO visitor_key_hints (keyring_id, hint, can_messages_chat, can_emails, can_contacts, can_relationship_sensitive, can_sensitive_private, llm_allow_owner_keys, llm_allow_server_keys)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)`,
		keyringID, hint, flags.CanMessagesChat, flags.CanEmails, flags.CanContacts, flags.CanRelationships, flags.CanSensitivePrivate, flags.LLMAllowOwnerKeys, flags.LLMAllowServerKeys); err != nil {
		return fmt.Errorf("save visitor hint: %w", err)
	}
	return tx.Commit()
}

// ListVisitorKeyHints returns hints for non-master keyring seats for this user.
func (s *SensitiveService) ListVisitorKeyHints(ctx context.Context) ([]model.VisitorKeyHint, error) {
	uid := appctx.UserIDFromCtx(ctx)
	var q string
	var args []any
	if uid > 0 {
		q = `
			SELECT h.id, h.keyring_id, h.hint, h.created_at,
				h.can_messages_chat, h.can_emails, h.can_contacts, h.can_relationship_sensitive, h.can_sensitive_private,
				h.llm_allow_owner_keys, h.llm_allow_server_keys
			FROM visitor_key_hints h
			INNER JOIN sensitive_keyring k ON k.id = h.keyring_id AND k.is_master = FALSE
			WHERE k.user_id = ?1
			ORDER BY h.created_at ASC`
		args = []any{uid}
	} else {
		q = `
			SELECT h.id, h.keyring_id, h.hint, h.created_at,
				h.can_messages_chat, h.can_emails, h.can_contacts, h.can_relationship_sensitive, h.can_sensitive_private,
				h.llm_allow_owner_keys, h.llm_allow_server_keys
			FROM visitor_key_hints h
			INNER JOIN sensitive_keyring k ON k.id = h.keyring_id AND k.is_master = FALSE
			WHERE k.user_id IS NULL
			ORDER BY h.created_at ASC`
	}
	rows, err := s.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.VisitorKeyHint
	for rows.Next() {
		var item model.VisitorKeyHint
		if err := rows.Scan(&item.ID, &item.KeyringID, &item.Hint, &item.CreatedAt,
			&item.CanMessagesChat, &item.CanEmails, &item.CanContacts, &item.CanRelationships, &item.CanSensitivePrivate,
			&item.LLMAllowOwnerKeys, &item.LLMAllowServerKeys); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListOrphanVisitorKeyringIDs returns visitor seat IDs that have no visitor_key_hints row for this user.
func (s *SensitiveService) ListOrphanVisitorKeyringIDs(ctx context.Context) ([]int64, error) {
	uid := appctx.UserIDFromCtx(ctx)
	var q string
	var args []any
	if uid > 0 {
		q = `
			SELECT k.id FROM sensitive_keyring k
			WHERE k.is_master = FALSE AND k.user_id = ?1
			AND NOT EXISTS (SELECT 1 FROM visitor_key_hints h WHERE h.keyring_id = k.id)
			ORDER BY k.id ASC`
		args = []any{uid}
	} else {
		q = `
			SELECT k.id FROM sensitive_keyring k
			WHERE k.is_master = FALSE AND k.user_id IS NULL
			AND NOT EXISTS (SELECT 1 FROM visitor_key_hints h WHERE h.keyring_id = k.id)
			ORDER BY k.id ASC`
	}
	rows, err := s.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// VisitorKeyRefDocPermissionRow is one task-eligible, non-sensitive reference document and
// whether the given visitor hint may expose it to LLM tools.
type VisitorKeyRefDocPermissionRow struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Allowed bool   `json:"allowed"`
}

func (s *SensitiveService) visitorHintOwnedByCtxUser(ctx context.Context, hintID int64) (bool, error) {
	uid := appctx.UserIDFromCtx(ctx)
	var exists bool
	var err error
	if uid > 0 {
		err = s.pool.QueryRowContext(ctx, `
			SELECT TRUE FROM visitor_key_hints h
			INNER JOIN sensitive_keyring k ON k.id = h.keyring_id AND k.is_master = FALSE
			WHERE h.id = ?1 AND k.user_id = ?2`, hintID, uid).Scan(&exists)
	} else {
		err = s.pool.QueryRowContext(ctx, `
			SELECT TRUE FROM visitor_key_hints h
			INNER JOIN sensitive_keyring k ON k.id = h.keyring_id AND k.is_master = FALSE
			WHERE h.id = ?1 AND k.user_id IS NULL`, hintID).Scan(&exists)
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ListVisitorKeyReferenceDocPermissions returns reference documents that are available_for_task
// and not sensitive for this archive, with Allowed set from the per-hint join table.
func (s *SensitiveService) ListVisitorKeyReferenceDocPermissions(ctx context.Context, hintID int64) ([]VisitorKeyRefDocPermissionRow, error) {
	ok, err := s.visitorHintOwnedByCtxUser(ctx, hintID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("visitor hint not found")
	}
	uid := appctx.UserIDFromCtx(ctx)
	var q string
	var args []any
	if uid > 0 {
		q = `
			SELECT d.id,
				COALESCE(NULLIF(TRIM(d.title), ''), d.filename),
				EXISTS (
					SELECT 1 FROM visitor_key_hint_reference_documents j
					WHERE j.visitor_key_hint_id = ?1 AND j.reference_document_id = d.id)
			FROM reference_documents d
			WHERE d.available_for_task = TRUE AND NOT d.include_in_system_prompt AND d.is_sensitive = FALSE AND d.user_id = ?2
			ORDER BY COALESCE(NULLIF(TRIM(d.title), ''), d.filename) ASC`
		args = []any{hintID, uid}
	} else {
		q = `
			SELECT d.id,
				COALESCE(NULLIF(TRIM(d.title), ''), d.filename),
				EXISTS (
					SELECT 1 FROM visitor_key_hint_reference_documents j
					WHERE j.visitor_key_hint_id = ?1 AND j.reference_document_id = d.id)
			FROM reference_documents d
			WHERE d.available_for_task = TRUE AND NOT d.include_in_system_prompt AND d.is_sensitive = FALSE AND d.user_id IS NULL
			ORDER BY COALESCE(NULLIF(TRIM(d.title), ''), d.filename) ASC`
		args = []any{hintID}
	}
	rows, err := s.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VisitorKeyRefDocPermissionRow
	for rows.Next() {
		var row VisitorKeyRefDocPermissionRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Allowed); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ReplaceVisitorKeyHintReferenceDocuments sets the full allowlist of reference document IDs for LLM tools
// for this visitor hint. IDs must be task-available, non-sensitive rows owned by the same archive user.
func (s *SensitiveService) ReplaceVisitorKeyHintReferenceDocuments(ctx context.Context, hintID int64, documentIDs []int64) error {
	ok, err := s.visitorHintOwnedByCtxUser(ctx, hintID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("visitor hint not found")
	}
	seen := map[int64]struct{}{}
	var uniq []int64
	for _, id := range documentIDs {
		if id <= 0 {
			continue
		}
		if _, dupe := seen[id]; dupe {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	uid := appctx.UserIDFromCtx(ctx)
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM visitor_key_hint_reference_documents WHERE visitor_key_hint_id = ?1`, hintID); err != nil {
		return err
	}
	for _, docID := range uniq {
		var inserted int64
		if uid > 0 {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO visitor_key_hint_reference_documents (visitor_key_hint_id, reference_document_id, user_id)
				SELECT ?1, d.id, ?3
				FROM reference_documents d
				WHERE d.id = ?2 AND d.available_for_task = TRUE AND NOT d.include_in_system_prompt AND d.is_sensitive = FALSE AND d.user_id = ?3
				RETURNING reference_document_id`, hintID, docID, uid).Scan(&inserted)
		} else {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO visitor_key_hint_reference_documents (visitor_key_hint_id, reference_document_id, user_id)
				SELECT ?1, d.id, NULL
				FROM reference_documents d
				WHERE d.id = ?2 AND d.available_for_task = TRUE AND NOT d.include_in_system_prompt AND d.is_sensitive = FALSE AND d.user_id IS NULL
				RETURNING reference_document_id`, hintID, docID).Scan(&inserted)
		}
		if err == sql.ErrNoRows {
			return fmt.Errorf("reference document %d is not eligible for this archive", docID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListVisitorKeySensitiveReferenceDocPermissions returns sensitive reference_documents rows
// (private/sensitive data records) for this archive with Allowed from the per-hint join table.
func (s *SensitiveService) ListVisitorKeySensitiveReferenceDocPermissions(ctx context.Context, hintID int64) ([]VisitorKeyRefDocPermissionRow, error) {
	ok, err := s.visitorHintOwnedByCtxUser(ctx, hintID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("visitor hint not found")
	}
	uid := appctx.UserIDFromCtx(ctx)
	var q string
	var args []any
	if uid > 0 {
		q = `
			SELECT d.id,
				COALESCE(NULLIF(TRIM(d.title), ''), d.filename),
				EXISTS (
					SELECT 1 FROM visitor_key_hint_sensitive_reference_documents j
					WHERE j.visitor_key_hint_id = ?1 AND j.reference_document_id = d.id)
			FROM reference_documents d
			WHERE d.is_sensitive = TRUE AND NOT d.include_in_system_prompt AND d.user_id = ?2
			ORDER BY COALESCE(NULLIF(TRIM(d.title), ''), d.filename) ASC`
		args = []any{hintID, uid}
	} else {
		q = `
			SELECT d.id,
				COALESCE(NULLIF(TRIM(d.title), ''), d.filename),
				EXISTS (
					SELECT 1 FROM visitor_key_hint_sensitive_reference_documents j
					WHERE j.visitor_key_hint_id = ?1 AND j.reference_document_id = d.id)
			FROM reference_documents d
			WHERE d.is_sensitive = TRUE AND NOT d.include_in_system_prompt AND d.user_id IS NULL
			ORDER BY COALESCE(NULLIF(TRIM(d.title), ''), d.filename) ASC`
		args = []any{hintID}
	}
	rows, err := s.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VisitorKeyRefDocPermissionRow
	for rows.Next() {
		var row VisitorKeyRefDocPermissionRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Allowed); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ReplaceVisitorKeyHintSensitiveReferenceDocuments sets the full allowlist of sensitive reference
// document IDs for LLM tools for this visitor hint. IDs must be is_sensitive rows owned by the archive.
func (s *SensitiveService) ReplaceVisitorKeyHintSensitiveReferenceDocuments(ctx context.Context, hintID int64, documentIDs []int64) error {
	ok, err := s.visitorHintOwnedByCtxUser(ctx, hintID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("visitor hint not found")
	}
	seen := map[int64]struct{}{}
	var uniq []int64
	for _, id := range documentIDs {
		if id <= 0 {
			continue
		}
		if _, dupe := seen[id]; dupe {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	uid := appctx.UserIDFromCtx(ctx)
	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM visitor_key_hint_sensitive_reference_documents WHERE visitor_key_hint_id = ?1`, hintID); err != nil {
		return err
	}
	for _, docID := range uniq {
		var inserted int64
		if uid > 0 {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO visitor_key_hint_sensitive_reference_documents (visitor_key_hint_id, reference_document_id, user_id)
				SELECT ?1, d.id, ?3
				FROM reference_documents d
				WHERE d.id = ?2 AND d.is_sensitive = TRUE AND NOT d.include_in_system_prompt AND d.user_id = ?3
				RETURNING reference_document_id`, hintID, docID, uid).Scan(&inserted)
		} else {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO visitor_key_hint_sensitive_reference_documents (visitor_key_hint_id, reference_document_id, user_id)
				SELECT ?1, d.id, NULL
				FROM reference_documents d
				WHERE d.id = ?2 AND d.is_sensitive = TRUE AND NOT d.include_in_system_prompt AND d.user_id IS NULL
				RETURNING reference_document_id`, hintID, docID).Scan(&inserted)
		}
		if err == sql.ErrNoRows {
			return fmt.Errorf("sensitive reference document %d is not eligible for this archive", docID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateVisitorKeyHint updates plain-text hint text for a visitor seat (by visitor_key_hints.id).
func (s *SensitiveService) UpdateVisitorKeyHint(ctx context.Context, hintID int64, hint string) error {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return fmt.Errorf("hint is required")
	}
	if len(hint) > maxVisitorKeyHintLen {
		return fmt.Errorf("hint exceeds %d characters", maxVisitorKeyHintLen)
	}
	tag, err := s.pool.ExecContext(ctx, `
		UPDATE visitor_key_hints SET hint = ?1
		WHERE id = ?2
		AND EXISTS (
			SELECT 1 FROM sensitive_keyring k
			WHERE k.id = visitor_key_hints.keyring_id AND k.is_master = FALSE
		)`, hint, hintID)
	if err != nil {
		return err
	}
	if sqlutil.RowsAffected(tag) == 0 {
		return fmt.Errorf("visitor hint not found")
	}
	return nil
}

// SetVisitorKeyFeatureFlags updates permission columns for a visitor_key_hints row owned by the current user.
func (s *SensitiveService) SetVisitorKeyFeatureFlags(ctx context.Context, hintID int64, f VisitorKeyFeatureFlags) error {
	uid := appctx.UserIDFromCtx(ctx)
	var err error
	var tag sql.Result
	if uid > 0 {
		tag, err = s.pool.ExecContext(ctx, `
			UPDATE visitor_key_hints v SET
				can_messages_chat = ?1,
				can_emails = ?2,
				can_contacts = ?3,
				can_relationship_sensitive = ?4,
				can_sensitive_private = ?5,
				llm_allow_owner_keys = ?6,
				llm_allow_server_keys = ?7
			FROM sensitive_keyring k
			WHERE v.id = ?8 AND v.keyring_id = k.id AND k.is_master = FALSE AND k.user_id = ?9`,
			f.CanMessagesChat, f.CanEmails, f.CanContacts, f.CanRelationships, f.CanSensitivePrivate, f.LLMAllowOwnerKeys, f.LLMAllowServerKeys, hintID, uid)
	} else {
		tag, err = s.pool.ExecContext(ctx, `
			UPDATE visitor_key_hints v SET
				can_messages_chat = ?1,
				can_emails = ?2,
				can_contacts = ?3,
				can_relationship_sensitive = ?4,
				can_sensitive_private = ?5,
				llm_allow_owner_keys = ?6,
				llm_allow_server_keys = ?7
			FROM sensitive_keyring k
			WHERE v.id = ?8 AND v.keyring_id = k.id AND k.is_master = FALSE AND k.user_id IS NULL`,
			f.CanMessagesChat, f.CanEmails, f.CanContacts, f.CanRelationships, f.CanSensitivePrivate, f.LLMAllowOwnerKeys, f.LLMAllowServerKeys, hintID)
	}
	if err != nil {
		return err
	}
	if sqlutil.RowsAffected(tag) == 0 {
		return fmt.Errorf("visitor hint not found")
	}
	return nil
}

// CreateVisitorKeyHintForOrphanSeat inserts a hint for a visitor keyring seat that has none yet.
func (s *SensitiveService) CreateVisitorKeyHintForOrphanSeat(ctx context.Context, keyringID int64, hint, masterPassword string, flags VisitorKeyFeatureFlags) error {
	ok, err := appcrypto.CheckSensitiveMasterPassword(ctx, s.pool, masterPassword, s.pepper)
	if err != nil {
		return fmt.Errorf("check master password: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid master password")
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return fmt.Errorf("hint is required")
	}
	if len(hint) > maxVisitorKeyHintLen {
		return fmt.Errorf("hint exceeds %d characters", maxVisitorKeyHintLen)
	}
	var isMaster bool
	err = s.pool.QueryRowContext(ctx, `SELECT is_master FROM sensitive_keyring WHERE id = ?1`, keyringID).Scan(&isMaster)
	if err != nil {
		return fmt.Errorf("keyring seat not found")
	}
	if isMaster {
		return fmt.Errorf("cannot attach hint to master seat")
	}
	var n int64
	if err := s.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM visitor_key_hints WHERE keyring_id = ?1`, keyringID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("hint already exists for this seat")
	}
	_, err = s.pool.ExecContext(ctx, `
		INSERT INTO visitor_key_hints (keyring_id, hint, can_messages_chat, can_emails, can_contacts, can_relationship_sensitive, can_sensitive_private, llm_allow_owner_keys, llm_allow_server_keys)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)`,
		keyringID, hint, flags.CanMessagesChat, flags.CanEmails, flags.CanContacts, flags.CanRelationships, flags.CanSensitivePrivate, flags.LLMAllowOwnerKeys, flags.LLMAllowServerKeys)
	return err
}

// DeleteVisitorSeatByHintID removes the visitor keyring seat linked to visitor_key_hints.id (hint row removed by CASCADE).
func (s *SensitiveService) DeleteVisitorSeatByHintID(ctx context.Context, hintID int64, masterPassword string) error {
	var keyringID int64
	err := s.pool.QueryRowContext(ctx, `SELECT keyring_id FROM visitor_key_hints WHERE id = ?1`, hintID).Scan(&keyringID)
	if err != nil {
		return fmt.Errorf("visitor hint not found")
	}
	return appcrypto.DeleteVisitorKeyringSeatByID(ctx, s.pool, keyringID, masterPassword, s.pepper)
}

// RemoveUser removes the keyring seat for userPassword. Requires masterPassword.
// Master seats cannot be removed.
func (s *SensitiveService) RemoveUser(ctx context.Context, userPassword, masterPassword string) error {
	return appcrypto.DeleteSensitiveKeyringSeat(ctx, s.pool, userPassword, masterPassword, s.pepper)
}

// RemoveAllVisitorKeys deletes every visitor (non-master) keyring seat for this user.
// Requires masterPassword that decrypts the owner master row. The master seat is preserved.
func (s *SensitiveService) RemoveAllVisitorKeys(ctx context.Context, masterPassword string) (removed int64, err error) {
	return appcrypto.DeleteAllVisitorKeyringSeats(ctx, s.pool, masterPassword, s.pepper)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (s *SensitiveService) toResponses(ctx context.Context, docs []*model.ReferenceDocument, password string) []model.SensitiveDataResponse {
	out := make([]model.SensitiveDataResponse, len(docs))
	hasKey := hasPassword(password)
	for i, doc := range docs {
		description := ""
		if doc.Title != nil {
			description = *doc.Title
		}
		details := redacted
		if hasKey {
			rawData, _, err := s.docRepo.GetData(ctx, doc.ID)
			if err == nil && len(rawData) > 0 {
				plain, err := appcrypto.DecryptDocumentData(ctx, s.pool, password, rawData, s.pepper)
				if err == nil && len(plain) > 0 {
					details = string(plain)
				}
			}
		} else {
			description = redacted
		}
		out[i] = model.SensitiveDataResponse{
			ID:                    doc.ID,
			Description:           description,
			Details:               details,
			IsPrivate:             doc.IsPrivate,
			IsSensitive:           doc.IsSensitive,
			IncludeInSystemPrompt: doc.IncludeInSystemPrompt,
			CreatedAt:             doc.CreatedAt.Format(time.RFC3339),
			UpdatedAt:             doc.UpdatedAt.Format(time.RFC3339),
		}
	}
	return out
}

func hasPassword(p string) bool {
	return strings.TrimSpace(p) != ""
}

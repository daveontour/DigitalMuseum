package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

const redacted = "*****************"

// SensitiveService handles sensitive-data CRUD and keyring management.
// Sensitive records are stored as reference_documents with is_sensitive=TRUE.
type SensitiveService struct {
	docRepo *repository.DocumentRepo
	pool    *pgxpool.Pool
	pepper  string
}

// NewSensitiveService creates a SensitiveService backed by DocumentRepo.
// pepper is ATTACHMENT_ALLOWED_TYPES from config (used for key derivation).
func NewSensitiveService(docRepo *repository.DocumentRepo, pool *pgxpool.Pool, pepper string) *SensitiveService {
	return &SensitiveService{docRepo: docRepo, pool: pool, pepper: pepper}
}

// Count returns the total number of sensitive records in reference_documents.
func (s *SensitiveService) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reference_documents WHERE is_sensitive = TRUE`).Scan(&n)
	return n, err
}

// KeyCount returns the total number of sensitive_keyring seats.
func (s *SensitiveService) KeyCount(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sensitive_keyring`).Scan(&n)
	return n, err
}

// VerifyMasterPassword returns true if masterPassword decrypts the master (is_master) keyring row.
func (s *SensitiveService) VerifyMasterPassword(ctx context.Context, masterPassword string) (bool, error) {
	return appcrypto.CheckSensitiveMasterPassword(ctx, s.pool, masterPassword, s.pepper)
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
func (s *SensitiveService) Create(ctx context.Context, masterPassword, description, details string, isPrivate, isSensitive bool) error {
	data := []byte(details)
	enc, err := appcrypto.EncryptDocumentData(ctx, s.pool, masterPassword, data, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt record: %w", err)
	}
	title := description
	_, err = s.docRepo.Create(ctx,
		description, "text/plain", int64(len(data)), enc,
		&title, nil, nil, nil, nil, nil,
		false, isPrivate, isSensitive, true,
	)
	return err
}

// Update re-encrypts details and updates the record.
func (s *SensitiveService) Update(ctx context.Context, id int64, masterPassword, description, details string, isPrivate, isSensitive bool) error {
	data := []byte(details)
	enc, err := appcrypto.EncryptDocumentData(ctx, s.pool, masterPassword, data, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt record: %w", err)
	}
	title := description
	if _, err := s.docRepo.Update(ctx, id, &title, nil, nil, nil, nil, nil, nil); err != nil {
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

// AddUser adds a new keyring seat for userPassword and stores a plain-text hint for the unlock dialog.
// hint must be non-empty (after trim).
func (s *SensitiveService) AddUser(ctx context.Context, userPassword, masterPassword, hint string) error {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return fmt.Errorf("visitor key hint is required")
	}
	if len(hint) > maxVisitorKeyHintLen {
		return fmt.Errorf("hint exceeds %d characters", maxVisitorKeyHintLen)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	keyringID, err := appcrypto.AddSensitiveKeyringSeatTx(ctx, tx, s.pool, userPassword, masterPassword, s.pepper)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO visitor_key_hints (keyring_id, hint) VALUES ($1, $2)`,
		keyringID, hint); err != nil {
		return fmt.Errorf("save visitor hint: %w", err)
	}
	return tx.Commit(ctx)
}

// ListVisitorKeyHints returns hints for non-master keyring seats (for visitor unlock UI).
func (s *SensitiveService) ListVisitorKeyHints(ctx context.Context) ([]model.VisitorKeyHint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.id, h.hint, h.created_at
		FROM visitor_key_hints h
		INNER JOIN sensitive_keyring k ON k.id = h.keyring_id AND k.is_master = FALSE
		ORDER BY h.created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.VisitorKeyHint
	for rows.Next() {
		var item model.VisitorKeyHint
		if err := rows.Scan(&item.ID, &item.Hint, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// RemoveUser removes the keyring seat for userPassword. Requires masterPassword.
// Master seats cannot be removed.
func (s *SensitiveService) RemoveUser(ctx context.Context, userPassword, masterPassword string) error {
	return appcrypto.DeleteSensitiveKeyringSeat(ctx, s.pool, userPassword, masterPassword, s.pepper)
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
			ID:          doc.ID,
			Description: description,
			Details:     details,
			IsPrivate:   doc.IsPrivate,
			IsSensitive: doc.IsSensitive,
			CreatedAt:   doc.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   doc.UpdatedAt.Format(time.RFC3339),
		}
	}
	return out
}

func hasPassword(p string) bool {
	return strings.TrimSpace(p) != ""
}

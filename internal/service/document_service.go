package service

import (
	"context"
	"errors"
	"fmt"

	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPasswordRequired is returned when an encrypted document is requested without a password.
var ErrPasswordRequired = errors.New("password required to access encrypted document")

// DocumentService orchestrates reference document operations.
type DocumentService struct {
	repo   *repository.DocumentRepo
	pool   *pgxpool.Pool
	pepper string
}

// NewDocumentService creates a DocumentService.
// pepper is used for keyring key derivation (same pepper used by SensitiveService).
func NewDocumentService(repo *repository.DocumentRepo, pool *pgxpool.Pool, pepper string) *DocumentService {
	return &DocumentService{repo: repo, pool: pool, pepper: pepper}
}

func (s *DocumentService) List(ctx context.Context, search, category, tag, contentType string, availableForTask *bool) ([]*model.ReferenceDocument, error) {
	return s.repo.List(ctx, search, category, tag, contentType, availableForTask)
}

func (s *DocumentService) GetByID(ctx context.Context, id int64) (*model.ReferenceDocument, error) {
	return s.repo.GetByID(ctx, id)
}

// GetData returns the raw (decrypted if needed) bytes for a document.
// If the document is encrypted and userPassword is empty, ErrPasswordRequired is returned.
// If userPassword is wrong (no matching keyring seat), nil bytes are returned.
func (s *DocumentService) GetData(ctx context.Context, id int64, userPassword string) ([]byte, error) {
	data, isEncrypted, err := s.repo.GetData(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isEncrypted {
		return data, nil
	}
	if userPassword == "" {
		return nil, ErrPasswordRequired
	}
	return appcrypto.DecryptDocumentData(ctx, s.pool, userPassword, data, s.pepper)
}

// Create stores a new reference document, optionally encrypting data with the keyring DEK.
// If masterPassword is non-empty the data is encrypted before storage.
func (s *DocumentService) Create(ctx context.Context,
	filename, contentType string, size int64, data []byte,
	title, description, author, tags, categories, notes *string,
	availableForTask, isPrivate, isSensitive bool, masterPassword string,
) (*model.ReferenceDocument, error) {
	isEncrypted := false
	if masterPassword != "" {
		enc, err := appcrypto.EncryptDocumentData(ctx, s.pool, masterPassword, data, s.pepper)
		if err != nil {
			return nil, fmt.Errorf("encrypt document data: %w", err)
		}
		data = enc
		isEncrypted = true
	}
	return s.repo.Create(ctx, filename, contentType, size, data,
		title, description, author, tags, categories, notes,
		availableForTask, isPrivate, isSensitive, isEncrypted)
}

func (s *DocumentService) Update(ctx context.Context, id int64,
	title, description, author, tags, categories, notes *string,
	availableForTask *bool,
) (*model.ReferenceDocument, error) {
	return s.repo.Update(ctx, id, title, description, author, tags, categories, notes, availableForTask)
}

func (s *DocumentService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// EncryptExisting encrypts all non-sensitive, non-encrypted documents using the keyring DEK.
// Returns the count of documents encrypted.
func (s *DocumentService) EncryptExisting(ctx context.Context, masterPassword string) (int, error) {
	docs, err := s.repo.ListUnencrypted(ctx)
	if err != nil {
		return 0, fmt.Errorf("list unencrypted documents: %w", err)
	}
	count := 0
	for _, doc := range docs {
		rawData, _, err := s.repo.GetData(ctx, doc.ID)
		if err != nil {
			return count, fmt.Errorf("get data for document %d: %w", doc.ID, err)
		}
		enc, err := appcrypto.EncryptDocumentData(ctx, s.pool, masterPassword, rawData, s.pepper)
		if err != nil {
			return count, fmt.Errorf("encrypt document %d: %w", doc.ID, err)
		}
		if err := s.repo.UpdateData(ctx, doc.ID, enc, true); err != nil {
			return count, fmt.Errorf("update document %d: %w", doc.ID, err)
		}
		count++
	}
	return count, nil
}

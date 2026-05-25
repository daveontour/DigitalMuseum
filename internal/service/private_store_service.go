package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// PrivateStoreService manages CRUD operations on the private_store table.
// All operations require the master password because entries are encrypted
// with the master-only private DEK.
type PrivateStoreService struct {
	repo   *repository.PrivateStoreRepo
	pool   *sql.DB
	pepper string
}

// NewPrivateStoreService creates a PrivateStoreService.
func NewPrivateStoreService(repo *repository.PrivateStoreRepo, pool *sql.DB, pepper string) *PrivateStoreService {
	return &PrivateStoreService{repo: repo, pool: pool, pepper: pepper}
}

// List returns all entries, decrypted with masterPassword.
func (s *PrivateStoreService) List(ctx context.Context, masterPassword string) ([]model.PrivateStoreResponse, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.PrivateStoreResponse, 0, len(rows))
	for _, row := range rows {
		plain, err := appcrypto.DecryptPrivateValue(ctx, s.pool, masterPassword, row.EncryptedValue, s.pepper)
		if err != nil {
			return nil, fmt.Errorf("decrypt entry %q: %w", row.Key, err)
		}
		out = append(out, model.PrivateStoreResponse{
			ID:        row.ID,
			Key:       row.Key,
			Value:     plain,
			CreatedAt: row.CreatedAt.Format(time.RFC3339),
			UpdatedAt: row.UpdatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// GetByKey returns a single decrypted entry, or nil if the key does not exist.
func (s *PrivateStoreService) GetByKey(ctx context.Context, key, masterPassword string) (*model.PrivateStoreResponse, error) {
	row, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	plain, err := appcrypto.DecryptPrivateValue(ctx, s.pool, masterPassword, row.EncryptedValue, s.pepper)
	if err != nil {
		return nil, fmt.Errorf("decrypt entry %q: %w", key, err)
	}
	resp := &model.PrivateStoreResponse{
		ID:        row.ID,
		Key:       row.Key,
		Value:     plain,
		CreatedAt: row.CreatedAt.Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.Format(time.RFC3339),
	}
	return resp, nil
}

// Create encrypts value and inserts a new entry.
func (s *PrivateStoreService) Create(ctx context.Context, key, value, masterPassword string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	enc, err := appcrypto.EncryptPrivateValue(ctx, s.pool, masterPassword, value, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt value: %w", err)
	}
	_, err = s.repo.Create(ctx, key, enc)
	return err
}

// Update re-encrypts value and replaces the stored entry.
func (s *PrivateStoreService) Update(ctx context.Context, key, value, masterPassword string) error {
	enc, err := appcrypto.EncryptPrivateValue(ctx, s.pool, masterPassword, value, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt value: %w", err)
	}
	return s.repo.Update(ctx, key, enc)
}

// Upsert encrypts value and inserts or updates the entry for key.
func (s *PrivateStoreService) Upsert(ctx context.Context, key, value, masterPassword string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	enc, err := appcrypto.EncryptPrivateValue(ctx, s.pool, masterPassword, value, s.pepper)
	if err != nil {
		return fmt.Errorf("encrypt value: %w", err)
	}
	return s.repo.Upsert(ctx, key, enc)
}

// Delete removes an entry. Requires a valid master password.
func (s *PrivateStoreService) Delete(ctx context.Context, key, masterPassword string) error {
	ok, err := appcrypto.CheckSensitiveMasterPassword(ctx, s.pool, masterPassword, s.pepper)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("invalid master password")
	}
	return s.repo.Delete(ctx, key)
}

// UpsertLLMToolsAccessPolicy stores encrypted policy for the owner and a plaintext mirror for visitor sessions.
func (s *PrivateStoreService) UpsertLLMToolsAccessPolicy(ctx context.Context, policyJSON, masterPassword string) error {
	if err := s.Upsert(ctx, appai.LLMToolsAccessStoreKey, policyJSON, masterPassword); err != nil {
		return err
	}
	return s.mirrorLLMToolsAccessPolicy(ctx, policyJSON)
}

// MirrorLLMToolsAccessPolicy writes the plaintext policy mirror (e.g. after loading from private_store).
func (s *PrivateStoreService) MirrorLLMToolsAccessPolicy(ctx context.Context, policyJSON string) error {
	return s.mirrorLLMToolsAccessPolicy(ctx, policyJSON)
}

func (s *PrivateStoreService) mirrorLLMToolsAccessPolicy(ctx context.Context, policyJSON string) error {
	if s.pool == nil {
		return fmt.Errorf("database not configured")
	}
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON == "" {
		return fmt.Errorf("policy JSON is required")
	}
	cfg := repository.NewConfigRepo(s.pool)
	desc := "LLM tool access policy mirror (visitor-readable)"
	_, err := cfg.Upsert(ctx, appai.LLMToolsAccessMirrorConfigKey, &policyJSON, nil, &desc)
	return err
}

// LoadLLMToolsAccessPolicyMirror reads the plaintext policy mirror for visitor-key sessions.
func (s *PrivateStoreService) LoadLLMToolsAccessPolicyMirror(ctx context.Context) (appai.ToolAccessPolicy, error) {
	if s.pool == nil {
		return nil, nil
	}
	cfg := repository.NewConfigRepo(s.pool)
	raw, err := cfg.GetValueByKey(ctx, appai.LLMToolsAccessMirrorConfigKey)
	if err != nil || raw == nil {
		return nil, err
	}
	return appai.ParseToolAccessPolicyJSON(*raw)
}

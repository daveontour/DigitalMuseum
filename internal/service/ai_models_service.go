package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// localAIModelKey is the special ai_models row representing Local AI (Ollama). Unlike every
// other row it is not routed through OpenRouter (model_slug is unused/blank), is seeded once
// automatically (see database.SeedAIModelsFromFileIfMissing) rather than user-created, and
// cannot be deleted — only enabled/disabled and reordered like any other row.
const localAIModelKey = "localai"

// AIModel is the in-memory representation of one admin-managed AI model row.
type AIModel struct {
	ID          int64
	Key         string
	DisplayName string
	ModelSlug   string
	Enabled     bool
	SortOrder   int
}

// AIModelInput holds the editable fields for one AI model.
type AIModelInput struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	ModelSlug   string `json:"model_slug"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

// AIModelsService manages the deployment-wide, admin-editable list of AI models
// that back the OpenRouter adapter. Reads are cached in-process (invalidated on
// every write) since chat requests resolve a model by key on every generation.
type AIModelsService struct {
	repo *repository.AIModelsRepo

	mu    sync.RWMutex
	cache []AIModel // nil = not loaded
}

// NewAIModelsService creates an AIModelsService.
func NewAIModelsService(repo *repository.AIModelsRepo) *AIModelsService {
	return &AIModelsService{repo: repo}
}

func toAIModel(row *model.AIModelRow) AIModel {
	return AIModel{
		ID:          row.ID,
		Key:         row.Key,
		DisplayName: row.DisplayName,
		ModelSlug:   row.ModelSlug,
		Enabled:     row.Enabled,
		SortOrder:   row.SortOrder,
	}
}

func (s *AIModelsService) refreshCache(ctx context.Context) ([]AIModel, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AIModel, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAIModel(row))
	}
	s.mu.Lock()
	s.cache = out
	s.mu.Unlock()
	return out, nil
}

func (s *AIModelsService) cached(ctx context.Context) ([]AIModel, error) {
	s.mu.RLock()
	if s.cache != nil {
		c := s.cache
		s.mu.RUnlock()
		return c, nil
	}
	s.mu.RUnlock()
	return s.refreshCache(ctx)
}

func (s *AIModelsService) invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// ListAll returns every model (admin table view) — always freshly read from
// the database so admin edits made elsewhere are reflected immediately.
func (s *AIModelsService) ListAll(ctx context.Context) ([]AIModel, error) {
	return s.refreshCache(ctx)
}

// ListEnabled returns enabled hosted (OpenRouter-routed) models ordered by sort_order, for
// runtime pickers such as chat provider dropdowns. Local AI is deliberately excluded — every
// consumer of this list already handles Local AI as a separate, always-available option;
// see defaultHostedLLMProviderOrder (internal/service/hosted_llm_order.go) for the Auto
// routing/error-failover order, which does include Local AI at its own sort position.
func (s *AIModelsService) ListEnabled(ctx context.Context) ([]AIModel, error) {
	all, err := s.cached(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AIModel, 0, len(all))
	for _, m := range all {
		if m.Enabled && strings.ToLower(m.Key) != localAIModelKey {
			out = append(out, m)
		}
	}
	return out, nil
}

// LocalAIRowEnabled reports whether the special "localai" row in ai_models is enabled.
// Fails open (true) when the row is missing (e.g. before the first seed has run), so Local
// AI is never silently blocked by a missing migration/seed.
func (s *AIModelsService) LocalAIRowEnabled(ctx context.Context) bool {
	all, err := s.cached(ctx)
	if err != nil {
		return true
	}
	for _, m := range all {
		if strings.ToLower(m.Key) == localAIModelKey {
			return m.Enabled
		}
	}
	return true
}

// GetByKey returns the model with the given key, only if enabled.
func (s *AIModelsService) GetByKey(ctx context.Context, key string) (*AIModel, bool) {
	all, err := s.cached(ctx)
	if err != nil {
		return nil, false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for _, m := range all {
		if strings.ToLower(m.Key) == key {
			if !m.Enabled {
				return nil, false
			}
			mm := m
			return &mm, true
		}
	}
	return nil, false
}

// IsEnabledKey reports whether key names a currently-enabled model.
func (s *AIModelsService) IsEnabledKey(ctx context.Context, key string) bool {
	_, ok := s.GetByKey(ctx, key)
	return ok
}

// DefaultKey returns the key of the first enabled model by sort_order, used
// wherever code needs a fallback provider when none is specified.
func (s *AIModelsService) DefaultKey(ctx context.Context) (string, bool) {
	enabled, err := s.ListEnabled(ctx)
	if err != nil || len(enabled) == 0 {
		return "", false
	}
	return enabled[0].Key, true
}

// Create inserts a new AI model row.
func (s *AIModelsService) Create(ctx context.Context, in AIModelInput) (*AIModel, error) {
	key, displayName, modelSlug, err := validateAIModelInput(in)
	if err != nil {
		return nil, err
	}
	exists, err := s.repo.KeyExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("conflict:ai model key already exists: %s", key)
	}
	row, err := s.repo.Create(ctx, key, displayName, modelSlug, in.Enabled, in.SortOrder)
	if err != nil {
		return nil, err
	}
	s.invalidate()
	m := toAIModel(row)
	return &m, nil
}

// Update replaces an existing AI model row.
func (s *AIModelsService) Update(ctx context.Context, id int64, in AIModelInput) (*AIModel, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	key, displayName, modelSlug, err := validateAIModelInput(in)
	if err != nil {
		return nil, err
	}
	conflict, err := s.repo.KeyExistsExcluding(ctx, key, id)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict:ai model key already exists: %s", key)
	}
	row, err := s.repo.Update(ctx, id, key, displayName, modelSlug, in.Enabled, in.SortOrder)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	s.invalidate()
	m := toAIModel(row)
	return &m, nil
}

// Delete removes an AI model row. The Local AI row can never be deleted — only disabled —
// since there's no "add it back" flow for it (unlike hosted models, which can be re-added
// from the Model Catalog).
func (s *AIModelsService) Delete(ctx context.Context, id int64) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing != nil && strings.ToLower(existing.Key) == localAIModelKey {
		return fmt.Errorf("conflict:Local AI cannot be deleted — disable it instead")
	}
	_, err = s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// validateAIModelInput validates Create/Update input. "auto" is always reserved (a UI
// meta-selector, never a real row). "localai" is allowed through with a blank model_slug —
// it's not routed through OpenRouter — but in practice only the automatic seed ever creates
// that row; KeyExists/KeyExistsExcluding below still block a user from creating a second one.
func validateAIModelInput(in AIModelInput) (key, displayName, modelSlug string, err error) {
	key = strings.ToLower(strings.TrimSpace(in.Key))
	if key == "" {
		return "", "", "", fmt.Errorf("key is required")
	}
	if key == "auto" {
		return "", "", "", fmt.Errorf("key %q is reserved", key)
	}
	displayName = strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return "", "", "", fmt.Errorf("display_name is required")
	}
	modelSlug = strings.TrimSpace(in.ModelSlug)
	if modelSlug == "" && key != localAIModelKey {
		return "", "", "", fmt.Errorf("model_slug is required")
	}
	return key, displayName, modelSlug, nil
}

package service

import (
	"context"
	"sync"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
)

const openRouterCatalogTTL = 10 * time.Minute

// OpenRouterCatalogService caches OpenRouter's public model catalog in-process
// (deployment-wide, not user-scoped) so the Configuration → Model Catalog tab
// doesn't re-fetch several hundred models on every open.
type OpenRouterCatalogService struct {
	mu        sync.RWMutex
	cache     []appai.OpenRouterCatalogModel
	fetchedAt time.Time
}

// NewOpenRouterCatalogService creates an OpenRouterCatalogService.
func NewOpenRouterCatalogService() *OpenRouterCatalogService {
	return &OpenRouterCatalogService{}
}

// List returns the cached catalog, refreshing it if stale or missing. On a
// refresh error, a stale cache (if any) is returned rather than failing.
func (s *OpenRouterCatalogService) List(ctx context.Context) ([]appai.OpenRouterCatalogModel, error) {
	s.mu.RLock()
	fresh := s.cache != nil && time.Since(s.fetchedAt) < openRouterCatalogTTL
	cached := s.cache
	s.mu.RUnlock()
	if fresh {
		return cached, nil
	}

	models, err := appai.FetchOpenRouterModels(ctx)
	if err != nil {
		if cached != nil {
			return cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	s.cache = models
	s.fetchedAt = time.Now()
	s.mu.Unlock()
	return models, nil
}

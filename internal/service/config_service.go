package service

import (
	"context"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// ConfigService manages app_configuration rows.
type ConfigService struct {
	repo *repository.ConfigRepo
}

// NewConfigService creates a ConfigService.
func NewConfigService(repo *repository.ConfigRepo) *ConfigService {
	return &ConfigService{repo: repo}
}

func (s *ConfigService) List(ctx context.Context) ([]*model.AppConfiguration, error) {
	return s.repo.List(ctx)
}

func (s *ConfigService) Upsert(ctx context.Context, key string, value *string, isMandatory *bool, description *string) (*model.AppConfiguration, error) {
	return s.repo.Upsert(ctx, key, value, isMandatory, description)
}

func (s *ConfigService) Delete(ctx context.Context, key string) (bool, error) {
	return s.repo.Delete(ctx, key)
}

func (s *ConfigService) SeedFromEnv(ctx context.Context) (int, error) {
	return s.repo.SeedFromEnv(ctx)
}

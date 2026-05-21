package service

import (
	"context"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// SavedResponseService manages saved chat responses.
type SavedResponseService struct {
	repo *repository.SavedResponseRepo
}

// NewSavedResponseService creates a SavedResponseService.
func NewSavedResponseService(repo *repository.SavedResponseRepo) *SavedResponseService {
	return &SavedResponseService{repo: repo}
}

func (s *SavedResponseService) List(ctx context.Context) ([]*model.SavedResponse, error) {
	return s.repo.List(ctx)
}

func (s *SavedResponseService) GetByID(ctx context.Context, id int64) (*model.SavedResponse, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *SavedResponseService) Create(ctx context.Context, title, content string, voice, llmProvider *string) (*model.SavedResponse, error) {
	return s.repo.Create(ctx, title, content, voice, llmProvider)
}

func (s *SavedResponseService) Update(ctx context.Context, id int64, title, content, voice, llmProvider *string) (*model.SavedResponse, error) {
	return s.repo.Update(ctx, id, title, content, voice, llmProvider)
}

func (s *SavedResponseService) Delete(ctx context.Context, id int64) (bool, error) {
	return s.repo.Delete(ctx, id)
}

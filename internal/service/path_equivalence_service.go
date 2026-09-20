package service

import (
	"context"
	"path/filepath"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// PathEquivalenceService manages filesystem-import path-equivalence rules.
// Request-shape validation (required fields, path_a != path_b) lives in the
// handler, matching the split used by e.g. ContactHandler.Create /
// ContactService.CreateContact — this service just normalizes paths.
type PathEquivalenceService struct {
	repo *repository.PathEquivalenceRepo
}

// NewPathEquivalenceService creates a PathEquivalenceService.
func NewPathEquivalenceService(repo *repository.PathEquivalenceRepo) *PathEquivalenceService {
	return &PathEquivalenceService{repo: repo}
}

func (s *PathEquivalenceService) List(ctx context.Context) ([]*model.PathEquivalence, error) {
	return s.repo.List(ctx)
}

func (s *PathEquivalenceService) Create(ctx context.Context, pathA, pathB string) (*model.PathEquivalence, error) {
	return s.repo.Create(ctx, filepath.Clean(pathA), filepath.Clean(pathB))
}

func (s *PathEquivalenceService) Update(ctx context.Context, id int64, pathA, pathB string) (*model.PathEquivalence, error) {
	return s.repo.Update(ctx, id, filepath.Clean(pathA), filepath.Clean(pathB))
}

func (s *PathEquivalenceService) Delete(ctx context.Context, id int64) (bool, error) {
	return s.repo.Delete(ctx, id)
}

package service

import (
	"context"
	"fmt"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
)

// InterestService manages interests.
type InterestService struct {
	repo *repository.InterestRepo
}

// NewInterestService creates an InterestService.
func NewInterestService(repo *repository.InterestRepo) *InterestService {
	return &InterestService{repo: repo}
}

func (s *InterestService) List(ctx context.Context) ([]*model.Interest, error) {
	return s.repo.List(ctx)
}

func (s *InterestService) GetByID(ctx context.Context, id int64) (*model.Interest, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *InterestService) Create(ctx context.Context, name string) (*model.Interest, error) {
	existing, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("conflict:interest already exists: %s", name)
	}
	return s.repo.Create(ctx, name)
}

func (s *InterestService) Update(ctx context.Context, id int64, name string) (*model.Interest, error) {
	conflict, err := s.repo.NameExistsExcluding(ctx, name, id)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict:interest already exists: %s", name)
	}
	return s.repo.Update(ctx, id, name)
}

func (s *InterestService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

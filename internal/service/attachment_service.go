package service

import (
	"context"

	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
)

// AttachmentService manages email attachment media items.
type AttachmentService struct {
	repo *repository.AttachmentRepo
}

// NewAttachmentService creates an AttachmentService.
func NewAttachmentService(repo *repository.AttachmentRepo) *AttachmentService {
	return &AttachmentService{repo: repo}
}

func (s *AttachmentService) GetRandom(ctx context.Context) (*model.AttachmentInfo, error) {
	return s.repo.GetRandom(ctx)
}

func (s *AttachmentService) GetByIDOrder(ctx context.Context, offset int) (*model.AttachmentInfo, error) {
	return s.repo.GetByIDOrder(ctx, offset)
}

func (s *AttachmentService) GetBySize(ctx context.Context, orderDesc bool, offset int) (*model.AttachmentInfo, error) {
	return s.repo.GetBySize(ctx, orderDesc, offset)
}

func (s *AttachmentService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}

func (s *AttachmentService) GetInfo(ctx context.Context, id int64) (*model.AttachmentInfo, error) {
	return s.repo.GetInfo(ctx, id)
}

func (s *AttachmentService) GetData(ctx context.Context, id int64) (data, thumbnail []byte, mediaType, filename string, err error) {
	return s.repo.GetData(ctx, id)
}

func (s *AttachmentService) Delete(ctx context.Context, id int64) (bool, error) {
	return s.repo.Delete(ctx, id)
}

func (s *AttachmentService) ListImages(ctx context.Context, page, pageSize int, order, direction string, allTypes bool) ([]*model.AttachmentInfo, int64, error) {
	return s.repo.ListImages(ctx, page, pageSize, order, direction, allTypes)
}

package service

import (
	"context"
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// quinielaService is the concrete implementation of QuinielaService.
type quinielaService struct {
	repo repository.QuinielaRepository
}

// NewQuinielaService constructs a quinielaService with the given dependencies.
func NewQuinielaService(repo repository.QuinielaRepository) QuinielaService {
	return &quinielaService{repo: repo}
}

func (s *quinielaService) Create(ctx context.Context, quiniela *domain.Quiniela) error {
	if err := domain.ValidateQuiniela(quiniela); err != nil {
		return err
	}
	return s.repo.Create(ctx, quiniela)
}

func (s *quinielaService) GetByID(ctx context.Context, id int) (*domain.Quiniela, error) {
	q, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", id))
	}
	return q, nil
}

func (s *quinielaService) GetByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error) {
	return s.repo.ListByOwner(ctx, ownerID)
}

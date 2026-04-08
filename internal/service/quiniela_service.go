package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// inviteCodeAlphabet uses an unambiguous character set: uppercase letters and
// digits with visually similar characters removed (0/O, 1/I/L) so codes are
// easy to read aloud or type from a screenshot.
const inviteCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
const inviteCodeLength = 10

// quinielaService is the concrete implementation of QuinielaService.
type quinielaService struct {
	repo       repository.QuinielaRepository
	memberRepo repository.GroupMembershipRepository
}

// NewQuinielaService constructs a quinielaService with the given dependencies.
func NewQuinielaService(repo repository.QuinielaRepository, memberRepo repository.GroupMembershipRepository) QuinielaService {
	return &quinielaService{repo: repo, memberRepo: memberRepo}
}

// generateInviteCode returns a cryptographically random invite code of
// inviteCodeLength characters drawn from inviteCodeAlphabet.
func generateInviteCode() (string, error) {
	b := make([]byte, inviteCodeLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	result := make([]byte, inviteCodeLength)
	for i, v := range b {
		result[i] = inviteCodeAlphabet[int(v)%len(inviteCodeAlphabet)]
	}
	return string(result), nil
}

func (s *quinielaService) Create(ctx context.Context, quiniela *domain.Quiniela) error {
	if err := domain.ValidateQuiniela(quiniela); err != nil {
		return err
	}

	code, err := generateInviteCode()
	if err != nil {
		return apperrors.Internal(err)
	}
	quiniela.InviteCode = code

	if quiniela.Currency == "" {
		quiniela.Currency = "MXN"
	}

	if err := s.repo.Create(ctx, quiniela); err != nil {
		return err
	}

	// Owner always becomes an active member immediately. They are marked as
	// paid automatically when the group is free (entry_fee = 0); for paid
	// groups the payment system will flip paid = true once confirmed.
	now := time.Now().UTC()
	ownerMembership := &domain.GroupMembership{
		QuinielaID: quiniela.ID,
		UserID:     quiniela.OwnerID,
		Status:     domain.MembershipActive,
		Paid:       quiniela.EntryFee == 0,
		JoinedAt:   &now,
	}
	return s.memberRepo.Create(ctx, ownerMembership)
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

func (s *quinielaService) GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error) {
	q, err := s.repo.GetByInviteCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound("group not found for the given invite code")
	}
	return q, nil
}

func (s *quinielaService) GetByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error) {
	return s.repo.ListByOwner(ctx, ownerID)
}

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
	repo repository.QuinielaRepository
}

// NewQuinielaService constructs a quinielaService with the given dependencies.
func NewQuinielaService(repo repository.QuinielaRepository) QuinielaService {
	return &quinielaService{repo: repo}
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
	if quiniela.PrizeThreshold == 0 {
		quiniela.PrizeThreshold = domain.DefaultPrizeThreshold
	}
	if err := domain.ValidateQuiniela(quiniela); err != nil {
		return err
	}

	code, err := generateInviteCode()
	if err != nil {
		return apperrors.Internal(err)
	}
	quiniela.InviteCode = code
	quiniela.InviteCodeExpiresAt = nil // invite links never expire

	// A new group starts inactive: the owner alone is below MinMembersForActive.
	// Status is promoted to active automatically when enough members join and
	// are approved.
	quiniela.Status = domain.QuinielaStatusInactive

	if quiniela.Currency == "" {
		quiniela.Currency = "MXN"
	}

	// Owner always becomes an active member immediately — no approval required.
	// Marked as paid for free groups; for paid groups the payment system will
	// flip paid=true after a confirmed transaction.
	// Both writes are wrapped in a single transaction via CreateWithMembership:
	// if the membership insert fails the quiniela row is rolled back, preventing
	// orphaned groups that have no owner membership.
	now := time.Now().UTC()
	ownerMembership := &domain.GroupMembership{
		UserID:   quiniela.OwnerID,
		Status:   domain.MembershipActive,
		Paid:     quiniela.EntryFee == 0,
		JoinedAt: &now,
	}
	return s.repo.CreateWithMembership(ctx, quiniela, ownerMembership)
}

// RotateInviteCode generates a new invite code for the quiniela, immediately
// invalidating the previous one. Only the quiniela owner may rotate the code;
// callers that are not the owner receive a 403 Forbidden error.
func (s *quinielaService) RotateInviteCode(ctx context.Context, quinielaID, ownerID int) (*domain.Quiniela, error) {
	q, err := s.repo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
	}
	if q.OwnerID != ownerID {
		return nil, apperrors.Forbidden("only the group owner can rotate the invite code")
	}

	newCode, err := generateInviteCode()
	if err != nil {
		return nil, apperrors.Internal(err)
	}

	// nil expiry: rotated codes also never expire, consistent with creation.
	return s.repo.RotateInviteCode(ctx, quinielaID, newCode, nil)
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

var _ QuinielaService = (*quinielaService)(nil)

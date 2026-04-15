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

// inviteCodeTTL is the default lifetime for a freshly generated invite code.
// After this period the code is considered expired and the owner must call
// RotateInviteCode to generate a new one. Keeping TTL finite prevents
// indefinite access when a code is shared beyond the intended audience.
const inviteCodeTTL = 30 * 24 * time.Hour // 30 days

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

	// Set a default expiry so leaked codes do not grant indefinite access.
	exp := time.Now().UTC().Add(inviteCodeTTL)
	quiniela.InviteCodeExpiresAt = &exp

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

	exp := time.Now().UTC().Add(inviteCodeTTL)
	return s.repo.RotateInviteCode(ctx, quinielaID, newCode, &exp)
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

package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ClerkEmail is a single address entry as transmitted by Clerk's webhook payload.
// Keeping this type in the service package decouples the JSON wire format
// (owned by the handler) from the business logic (owned by the service).
type ClerkEmail struct {
	ID      string
	Address string
}

// ClerkUserSyncer handles user synchronisation from Clerk webhook events.
// The implementation is responsible for email resolution, name normalisation,
// and idempotent DB persistence (create-or-update). The handler is responsible
// only for signature verification and JSON parsing.
type ClerkUserSyncer interface {
	Upsert(ctx context.Context, subject, firstName, lastName, primaryEmailID string, emails []ClerkEmail) error
}

type clerkUserSyncService struct {
	userRepo repository.UserRepository
	log      *zap.Logger
}

// NewClerkUserSyncService constructs the canonical ClerkUserSyncer.
func NewClerkUserSyncService(userRepo repository.UserRepository, log *zap.Logger) ClerkUserSyncer {
	return &clerkUserSyncService{userRepo: userRepo, log: log}
}

// Upsert creates or updates an internal User from a Clerk user.created /
// user.updated webhook payload. It resolves the primary email, validates it,
// normalises the display name, and delegates to the repository.
func (s *clerkUserSyncService) Upsert(ctx context.Context, subject, firstName, lastName, primaryEmailID string, emails []ClerkEmail) error {
	email := s.resolvePrimaryEmail(emails, primaryEmailID)
	if email != "" {
		if err := domain.ValidateEmail(email); err != nil {
			return apperrors.Validation("webhook payload contains an invalid email address")
		}
	}

	name := strings.TrimSpace(firstName + " " + lastName)
	if name == "" {
		name = subject
	}

	existing, err := s.userRepo.GetByClerkSubject(ctx, subject)
	if err != nil {
		return apperrors.Internal(err)
	}

	if existing != nil {
		existing.Name = name
		existing.Email = email
		existing.ClerkSubject = subject
		if err := s.userRepo.Update(ctx, existing); err != nil {
			return err
		}
		s.log.Info("clerk sync: updated user",
			zap.Int("user_id", existing.ID),
			zap.String("clerk_subject", subject),
		)
		return nil
	}

	user := &domain.User{
		Name:         name,
		Email:        email,
		ClerkSubject: subject,
		Role:         domain.RoleUser,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return err
	}
	s.log.Info("clerk sync: created user",
		zap.Int("user_id", user.ID),
		zap.String("clerk_subject", subject),
	)
	return nil
}

// resolvePrimaryEmail returns the email address whose ID matches primaryEmailID.
//
// When primaryEmailID is non-empty but no matching entry is found, Clerk is in
// a transient eventual-consistency state (the primary pointer was updated before
// the address list propagated). A warning is logged so operators can correlate
// it with Clerk delivery logs, and the first available address is used as a
// safe fallback — user creation must not fail on a missing primary pointer.
//
// When primaryEmailID is empty (e.g. OAuth users without a verified email) the
// first address is used without a warning.
func (s *clerkUserSyncService) resolvePrimaryEmail(emails []ClerkEmail, primaryEmailID string) string {
	for _, e := range emails {
		if e.ID == primaryEmailID {
			return e.Address
		}
	}
	if primaryEmailID != "" && len(emails) > 0 {
		s.log.Warn("clerk sync: primary_email_address_id not matched; falling back to first address",
			zap.String("primary_email_address_id", primaryEmailID),
			zap.Int("address_count", len(emails)),
		)
	}
	if len(emails) > 0 {
		return emails[0].Address
	}
	return ""
}

var _ ClerkUserSyncer = (*clerkUserSyncService)(nil)

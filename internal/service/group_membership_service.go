package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// GroupMembershipService manages user membership in Quinielas.
//
// Join resolves the invite code and creates a pending join request inside a
// single repository transaction - the user is NOT active until any existing
// active member calls ApproveJoin. ListByQuiniela returns the full roster.
// ListByUser returns all groups a user belongs to, regardless of status.
//
// ApproveJoin promotes a pending request to active. Any active member of the
// quiniela may approve - there is no admin-only gate. After approval the group
// status is synchronised: if active member count reaches MinMembersForActive
// the quiniela transitions from inactive to active.
//
// Leave lets a user remove themselves from a quiniela. Only the user themselves
// may call this; no admin or owner can remove another member. After leaving,
// the group status is re-evaluated and may become inactive.
//
// MarkPaid is called exclusively by the payment system after a transaction is
// confirmed. It must never be exposed as a direct API action - callers cannot
// mark themselves as paid. For free groups (entry_fee = 0), paid is set to
// true automatically at join time and this method is never invoked.
type GroupMembershipService interface {
	Join(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error)
	// JoinWithBalance joins the quiniela and immediately deducts the entry fee
	// from the user's balance, marking the membership as paid in one atomic
	// step. Returns Conflict when the user has insufficient available balance
	// or when the quiniela has no entry fee.
	JoinWithBalance(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error)
	ApproveJoin(ctx context.Context, quinielaID, membershipID, approverUserID int) (*domain.GroupMembership, error)
	Leave(ctx context.Context, quinielaID, callerUserID int) error
	MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error)
}

// GroupMembershipOption configures optional behaviour of groupMembershipService.
type GroupMembershipOption func(*groupMembershipService)

// WithGroupMembershipOutboxWriter enables outbox-based fan-out notifications for
// EventGroupMemberJoined and EventGroupMemberLeft. When nil, no outbox entries
// are written (silent no-op).
func WithGroupMembershipOutboxWriter(w outbox.Writer) GroupMembershipOption {
	return func(s *groupMembershipService) { s.outboxWriter = w }
}

// groupMembershipService is the concrete implementation of GroupMembershipService.
type groupMembershipService struct {
	quinielaRepo repository.QuinielaRepository
	memberRepo   repository.GroupMembershipRepository
	authz        GroupAuthz
	params       SystemParamService
	audit        AuditLogger
	paymentSvc   PaymentService
	clock        clock.Nower
	log          *zap.Logger
	outboxWriter outbox.Writer // nil ⇒ fan-out events not published
}

// NewGroupMembershipService constructs a groupMembershipService.
// GroupAuthz is derived from memberRepo so callers do not need to wire it
// separately; the same repository is always the source of truth for both
// data operations and permission checks within this service.
// Pass WithGroupMembershipOutboxWriter to enable member-joined/left fan-out events.
func NewGroupMembershipService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	params SystemParamService,
	audit AuditLogger,
	paymentSvc PaymentService,
	clk clock.Nower,
	log *zap.Logger,
	opts ...GroupMembershipOption,
) GroupMembershipService {
	svc := &groupMembershipService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
		authz:        NewGroupAuthzService(memberRepo),
		params:       params,
		audit:        audit,
		paymentSvc:   paymentSvc,
		clock:        clk,
		log:          log,
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// Join resolves invite_code to a Quiniela and creates a pending join request.
// The user is NOT active until any existing active member calls ApproveJoin.
// If the user was previously a member but left, they are re-queued as pending
// for a new approval round.
func (s *groupMembershipService) Join(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error) {
	maxMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMaxSize, domain.MaxMembersPerGroup)
	quiniela, m, err := s.memberRepo.RequestJoinByInviteCode(ctx, inviteCode, userID, maxMembers)
	if err != nil {
		return nil, err
	}
	if quiniela.EntryFee > 0 {
		s.createPendingPayment(ctx, quiniela, userID)
	}
	return m, nil
}

// createPendingPayment creates a payment_record with status=pending for the
// joining user. Errors are logged and swallowed: the membership is already
// persisted and a missing payment record is a recoverable admin concern, not
// a reason to roll back the join request.
func (s *groupMembershipService) createPendingPayment(ctx context.Context, quiniela *domain.Quiniela, userID int) {
	if _, err := s.paymentSvc.CreateRecord(ctx, quiniela.ID, userID, quiniela.EntryFee, quiniela.Currency, ""); err != nil {
		s.log.Warn("membership: failed to create pending payment record on join",
			zap.Int("quiniela_id", quiniela.ID),
			zap.Int("user_id", userID),
			zap.Error(err),
		)
	}
}

// JoinWithBalance joins the quiniela and immediately pays the entry fee from
// the user's balance. The balance deduction and paid=true flag are set
// atomically. The membership remains pending until an existing member approves.
func (s *groupMembershipService) JoinWithBalance(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error) {
	maxMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMaxSize, domain.MaxMembersPerGroup)
	quiniela, m, err := s.memberRepo.RequestJoinByInviteCode(ctx, inviteCode, userID, maxMembers)
	if err != nil {
		return nil, err
	}
	if quiniela.EntryFee <= 0 {
		return nil, apperrors.Conflict("this quiniela has no entry fee — use the standard join endpoint")
	}

	m2, err := s.memberRepo.DebitBalanceAndMarkPaid(ctx, quiniela.ID, userID, quiniela.EntryFee)
	if err != nil {
		return nil, err
	}

	resType := "membership"
	s.audit.Log(ctx, &userID, nil, domain.AuditActionBalanceDebited, &resType, &m2.ID, map[string]any{
		"quiniela_id":  quiniela.ID,
		"amount_cents": quiniela.EntryFee,
		"currency":     quiniela.Currency,
	})
	return m, nil
}

// ApproveJoin promotes a pending membership to active. The approverUserID must
// be an active member of the same quiniela - any member may approve; there is
// no admin-only gate. The membership update and group status recalculation are
// committed atomically via ApproveMembership.
func (s *groupMembershipService) ApproveJoin(ctx context.Context, quinielaID, membershipID, approverUserID int) (*domain.GroupMembership, error) {
	if err := s.authz.RequireActiveMember(ctx, quinielaID, approverUserID); err != nil {
		return nil, err
	}

	pending, err := s.memberRepo.GetByID(ctx, membershipID)
	if err != nil {
		return nil, err
	}
	if pending == nil || pending.QuinielaID != quinielaID {
		return nil, apperrors.NotFound("join request not found")
	}
	if pending.Status != domain.MembershipPending {
		return nil, apperrors.Conflict("this join request is no longer pending")
	}

	minMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMinMembers, domain.MinMembersForActive)
	maxMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMaxSize, domain.MaxMembersPerGroup)
	m, err := s.memberRepo.ApproveMembership(ctx, membershipID, quinielaID, s.clock.Now(), minMembers, maxMembers)
	if err != nil {
		return nil, err
	}

	resType := "membership"
	s.audit.Log(ctx, &approverUserID, nil, domain.AuditActionJoinApproved, &resType, &m.ID, map[string]any{
		"quiniela_id":  quinielaID,
		"approved_uid": m.UserID,
	})
	s.writeMembershipEvent(ctx, notification.EventGroupMemberJoined, quinielaID, m.UserID, m.ID)
	return m, nil
}

// Leave sets the caller's own membership to left. Only the member themselves
// may call this - no admin or owner can remove another user. If the leaving
// user holds MembershipRoleCreateOwner, ownership is transferred to the oldest
// remaining active member within the same transaction as the leave operation.
func (s *groupMembershipService) Leave(ctx context.Context, quinielaID, callerUserID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerUserID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != domain.MembershipActive {
		return apperrors.Validation("you are not an active member of this group")
	}

	minMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMinMembers, domain.MinMembersForActive)
	if m.Role == domain.MembershipRoleCreateOwner {
		successor, err := s.memberRepo.OldestActiveMember(ctx, quinielaID, callerUserID)
		if err != nil {
			return err
		}
		if successor != nil {
			if err := s.memberRepo.LeaveMembershipAndTransferOwnership(
				ctx,
				quinielaID,
				callerUserID,
				successor.ID,
				s.clock.Now(),
				minMembers,
			); err != nil {
				return err
			}
			s.writeMembershipEvent(ctx, notification.EventGroupMemberLeft, quinielaID, callerUserID, m.ID)
			return nil
		}
	}

	if err := s.memberRepo.LeaveMembership(ctx, quinielaID, callerUserID, s.clock.Now(), minMembers); err != nil {
		return err
	}
	s.writeMembershipEvent(ctx, notification.EventGroupMemberLeft, quinielaID, callerUserID, m.ID)
	return nil
}

// MarkPaid flips the paid flag to true for the given membership. It is
// intended to be called by the payment system after a successful transaction
// - never from an HTTP handler directly.
func (s *groupMembershipService) MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error) {
	m, err := s.memberRepo.MarkPaid(ctx, quinielaID, userID)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *groupMembershipService) ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error) {
	members, err := s.memberRepo.ListByQuiniela(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (s *groupMembershipService) ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error) {
	return s.memberRepo.ListByUser(ctx, userID)
}

// writeMembershipEvent publishes an outbox fan-out event for group membership
// changes. It looks up the quiniela to include name and owner_id in the
// payload, which the UserDispatcher needs for notification content rendering.
// Errors are logged and swallowed — the domain operation already committed and
// a best-effort notification failure must not roll it back.
func (s *groupMembershipService) writeMembershipEvent(
	ctx context.Context,
	eventType notification.EventType,
	quinielaID, actorUserID, membershipID int,
) {
	if s.outboxWriter == nil {
		return
	}
	quiniela, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil || quiniela == nil {
		s.log.Warn("membership: quiniela lookup for outbox event failed (best-effort)",
			zap.String("event_type", string(eventType)),
			zap.Int("quiniela_id", quinielaID),
			zap.Error(err),
		)
		quiniela = &domain.Quiniela{ID: quinielaID}
	}
	payload := notification.GroupJoinPayload{
		QuinielaID:   quiniela.ID,
		QuinielaName: quiniela.Name,
		MembershipID: membershipID,
		UserID:       actorUserID,
		OwnerID:      quiniela.OwnerID,
	}
	if wErr := s.outboxWriter.Write(ctx, eventType, "quiniela", fmt.Sprintf("%d", quinielaID), payload); wErr != nil {
		s.log.Warn("membership: outbox write failed (best-effort)",
			zap.String("event_type", string(eventType)),
			zap.Int("quiniela_id", quinielaID),
			zap.Error(wErr),
		)
	}
}

// enforce compile-time interface satisfaction.
var _ GroupMembershipService = (*groupMembershipService)(nil)

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	adminGroupNilErrFmt    = "expected nil error, got %v"
	adminGroupExpectErrMsg = "expected error, got nil"
)

// noopSnapshotter implements Snapshotter for tests where snapshot content is irrelevant.
type noopSnapshotter struct{}

func (s *noopSnapshotter) Snapshot(_ context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error) {
	return &domain.LeaderboardSnapshot{QuinielaID: quinielaID}, nil
}
func (s *noopSnapshotter) SnapshotForMatch(_ context.Context, quinielaID, _ int) (*domain.LeaderboardSnapshot, error) {
	return &domain.LeaderboardSnapshot{QuinielaID: quinielaID}, nil
}

// noopRanker implements Ranker for tests where leaderboard content is irrelevant.
type noopRanker struct {
	result *LeaderboardResult
	err    error
}

func (r *noopRanker) GetLeaderboard(_ context.Context, _ int) (*LeaderboardResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.result != nil {
		return r.result, nil
	}
	return &LeaderboardResult{}, nil
}
func (r *noopRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) (*LeaderboardResult, error) {
	return &LeaderboardResult{}, r.err
}

func newAdminGroupSvc(qr *stubQuinielaRepo, mr *stubMemberRepo) AdminGroupService {
	return NewAdminGroupService(qr, mr, &noopSnapshotter{}, &noopRanker{}, &noopAuditLogger{}, zap.NewNop())
}

// ── DeleteGroup ───────────────────────────────────────────────────────────────

func TestAdminGroupService_DeleteGroup_HappyPath_ReturnsNil(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if err := svc.DeleteGroup(context.Background(), 1, 99); err != nil {
		t.Errorf(adminGroupNilErrFmt, err)
	}
}

func TestAdminGroupService_DeleteGroup_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("not found")}, &stubMemberRepo{})

	if err := svc.DeleteGroup(context.Background(), 1, 99); err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── RemoveMember ──────────────────────────────────────────────────────────────

func TestAdminGroupService_RemoveMember_HappyPath_ReturnsNil(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if err := svc.RemoveMember(context.Background(), 10, 99); err != nil {
		t.Errorf(adminGroupNilErrFmt, err)
	}
}

func TestAdminGroupService_RemoveMember_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New("inactive")})

	if err := svc.RemoveMember(context.Background(), 10, 99); err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── UpdateGroupSettings ───────────────────────────────────────────────────────

func TestAdminGroupService_UpdateGroupSettings_ReturnsQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test", EntryFee: 500}
	svc := newAdminGroupSvc(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.UpdateGroupSettings(context.Background(), 1, 500, 99)
	if err != nil || got == nil {
		t.Fatalf("expected quiniela, got %v err=%v", got, err)
	}
}

func TestAdminGroupService_UpdateGroupSettings_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("not found")}, &stubMemberRepo{})

	_, err := svc.UpdateGroupSettings(context.Background(), 1, 0, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── TransferOwnership ─────────────────────────────────────────────────────────

func TestAdminGroupService_TransferOwnership_HappyPath_DemotesAndPromotes(t *testing.T) {
	newOwner := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipActive, Role: domain.MembershipRoleMember}
	mr := &stubMemberRepo{membership: newOwner}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	if err := svc.TransferOwnership(context.Background(), 1, 2, 99); err != nil {
		t.Errorf(adminGroupNilErrFmt, err)
	}
}

func TestAdminGroupService_TransferOwnership_NewOwnerNotMember_ReturnsNotFound(t *testing.T) {
	mr := &stubMemberRepo{membership: nil}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdminGroupService_TransferOwnership_NewOwnerInactive_ReturnsNotFound(t *testing.T) {
	inactive := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipLeft}
	mr := &stubMemberRepo{membership: inactive}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdminGroupService_TransferOwnership_TransferRolesError_Propagates(t *testing.T) {
	newOwner := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipActive}
	svc := NewAdminGroupService(&stubQuinielaRepo{}, &errOnTransferOwnershipRepo{newOwner: newOwner}, &noopSnapshotter{}, &noopRanker{}, &noopAuditLogger{}, zap.NewNop())

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if err == nil {
		t.Error("expected error from TransferOwnershipRoles, got nil")
	}
}

// ── BulkDeleteGroups ──────────────────────────────────────────────────────────

func TestAdminGroupService_BulkDeleteGroups_AllSucceed_ReturnsAllInSucceeded(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	result, err := svc.BulkDeleteGroups(context.Background(), []int{1, 2, 3}, 99)
	if err != nil {
		t.Fatalf(adminGroupNilErrFmt, err)
	}
	if len(result.Succeeded) != 3 {
		t.Errorf("expected 3 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestAdminGroupService_BulkDeleteGroups_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("db error")}, &stubMemberRepo{})

	_, err := svc.BulkDeleteGroups(context.Background(), []int{1, 2}, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── BulkRemoveMembers ─────────────────────────────────────────────────────────

func TestAdminGroupService_BulkRemoveMembers_AllSucceed_ReturnsAllInSucceeded(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	result, err := svc.BulkRemoveMembers(context.Background(), 1, []int{10, 11}, 99)
	if err != nil {
		t.Fatalf(adminGroupNilErrFmt, err)
	}
	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestAdminGroupService_BulkRemoveMembers_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New("db error")})

	_, err := svc.BulkRemoveMembers(context.Background(), 1, []int{10}, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── RecalculateLeaderboard ────────────────────────────────────────────────────

func TestAdminGroupService_RecalculateLeaderboard_HappyPath_ReturnsSnapshot(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	snap, err := svc.RecalculateLeaderboard(context.Background(), 5, 99)
	if err != nil {
		t.Fatalf(adminGroupNilErrFmt, err)
	}
	if snap == nil || snap.QuinielaID != 5 {
		t.Errorf("expected snapshot with QuinielaID=5, got %v", snap)
	}
}

func TestAdminGroupService_RecalculateLeaderboard_SnapshotterError_Propagates(t *testing.T) {
	errSnap := &errSnapshotter{}
	svc := NewAdminGroupService(&stubQuinielaRepo{}, &stubMemberRepo{}, errSnap, &noopRanker{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.RecalculateLeaderboard(context.Background(), 5, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// errSnapshotter always fails Snapshot.
type errSnapshotter struct{}

func (*errSnapshotter) Snapshot(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
	return nil, errors.New("snapshotter error")
}
func (*errSnapshotter) SnapshotForMatch(_ context.Context, _, _ int) (*domain.LeaderboardSnapshot, error) {
	return nil, errors.New("snapshotter error")
}

// errOnTransferOwnershipRepo returns the newOwner from GetByQuinielaAndUser but
// fails TransferOwnershipRoles, simulating a transaction error mid-transfer.
type errOnTransferOwnershipRepo struct {
	stubMemberRepo
	newOwner *domain.GroupMembership
}

func (r *errOnTransferOwnershipRepo) GetByQuinielaAndUser(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.newOwner, nil
}
func (r *errOnTransferOwnershipRepo) TransferOwnershipRoles(_ context.Context, _, _ int) error {
	return errors.New("db error")
}

// ── DistributePrizes ──────────────────────────────────────────────────────────

// captureDistributeRepo is a stubQuinielaRepo variant that captures the slices
// passed to DistributePrizesAtomically so tests can assert on classification.
type captureDistributeRepo struct {
	quiniela     *domain.Quiniela
	err          error
	onDistribute func(credits []repository.PrizeCredit, freezes []repository.PrizeFreeze)
}

func (r *captureDistributeRepo) CreateWithMembership(_ context.Context, _ *domain.Quiniela, _ *domain.GroupMembership) error {
	return r.err
}
func (r *captureDistributeRepo) Create(_ context.Context, _ *domain.Quiniela) error { return r.err }
func (r *captureDistributeRepo) GetByID(_ context.Context, _ int) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *captureDistributeRepo) GetByInviteCode(_ context.Context, _ string) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *captureDistributeRepo) Update(_ context.Context, _ *domain.Quiniela) error { return r.err }
func (r *captureDistributeRepo) Delete(_ context.Context, _ int) error              { return r.err }
func (r *captureDistributeRepo) ListByOwner(_ context.Context, _ int) ([]*domain.Quiniela, error) {
	return nil, r.err
}
func (r *captureDistributeRepo) RotateInviteCode(_ context.Context, _ int, _ string, _ *time.Time) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *captureDistributeRepo) UpdateStatus(_ context.Context, _ int, _ domain.QuinielaStatus) error {
	return r.err
}
func (r *captureDistributeRepo) UpdateGroupSettings(_ context.Context, _, _ int) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *captureDistributeRepo) DeleteByAdmin(_ context.Context, _, _ int) error { return r.err }
func (r *captureDistributeRepo) ListByIDs(_ context.Context, _ []int) ([]*domain.Quiniela, error) {
	return nil, r.err
}
func (r *captureDistributeRepo) GetStatusCounts(_ context.Context) (repository.QuinielaStatusCounts, error) {
	return repository.QuinielaStatusCounts{}, r.err
}
func (r *captureDistributeRepo) BulkDeleteByAdmin(_ context.Context, ids []int, _ int) ([]int, error) {
	return ids, r.err
}
func (r *captureDistributeRepo) DistributePrizesAtomically(_ context.Context, _, _ int, credits []repository.PrizeCredit, freezes []repository.PrizeFreeze) error {
	if r.err != nil {
		return r.err
	}
	if r.onDistribute != nil {
		r.onDistribute(credits, freezes)
	}
	return nil
}

type stubPrizeCrediter struct {
	credited bool
	err      error
}

func (s *stubPrizeCrediter) CreditPrize(_ context.Context, _, _ int, _ int64, _ string) (bool, error) {
	return s.credited, s.err
}

func newDistributeSvc(q *stubQuinielaRepo, ranker *noopRanker, prize *stubPrizeCrediter) AdminGroupService {
	svc := NewAdminGroupService(q, &stubMemberRepo{}, &noopSnapshotter{}, ranker, &noopAuditLogger{}, zap.NewNop())
	if prize != nil {
		svc.(*adminGroupService).SetPrizeCrediter(prize)
	}
	return svc
}

func eligibleLeaderboard(userIDs ...int) *LeaderboardResult {
	entries := make([]*domain.LeaderboardEntry, len(userIDs))
	for i, uid := range userIDs {
		entries[i] = &domain.LeaderboardEntry{
			User:        &domain.User{ID: uid, KYCTier: domain.KYCTierTwo},
			PrizeWinner: true,
		}
	}
	return &LeaderboardResult{
		Entries:           entries,
		ActivePaidMembers: len(userIDs),
		WinnerCount:       len(userIDs),
		EligibleForPrizes: true,
	}
}

func eligibleLeaderboardTier0(userIDs ...int) *LeaderboardResult {
	entries := make([]*domain.LeaderboardEntry, len(userIDs))
	for i, uid := range userIDs {
		entries[i] = &domain.LeaderboardEntry{
			User:        &domain.User{ID: uid, KYCTier: domain.KYCTierUnverified},
			PrizeWinner: true,
		}
	}
	return &LeaderboardResult{
		Entries:           entries,
		ActivePaidMembers: len(userIDs),
		WinnerCount:       len(userIDs),
		EligibleForPrizes: true,
	}
}

func TestAdminGroupService_DistributePrizes_NoPrizeCrediter_ReturnsError(t *testing.T) {
	q := &stubQuinielaRepo{quiniela: &domain.Quiniela{ID: 1, EntryFee: 1000}}
	svc := newDistributeSvc(q, &noopRanker{}, nil) // no crediter wired
	if err := svc.DistributePrizes(context.Background(), 1, 99); err == nil {
		t.Fatal("expected error when prize crediter not configured, got nil")
	}
}

func TestAdminGroupService_DistributePrizes_GroupNotFound_ReturnsNotFound(t *testing.T) {
	q := &stubQuinielaRepo{quiniela: nil}
	svc := newDistributeSvc(q, &noopRanker{}, &stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 99, 1); err == nil {
		t.Fatal("expected not-found, got nil")
	}
}

func TestAdminGroupService_DistributePrizes_ZeroEntryFee_ReturnsConflict(t *testing.T) {
	q := &stubQuinielaRepo{quiniela: &domain.Quiniela{ID: 1, EntryFee: 0}}
	svc := newDistributeSvc(q, &noopRanker{}, &stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 1, 1); err == nil {
		t.Fatal("expected conflict for zero entry fee, got nil")
	}
}

func TestAdminGroupService_DistributePrizes_NotEligible_ReturnsConflict(t *testing.T) {
	q := &stubQuinielaRepo{quiniela: &domain.Quiniela{ID: 1, EntryFee: 1000}}
	ranker := &noopRanker{result: &LeaderboardResult{EligibleForPrizes: false, WinnerCount: 0}}
	svc := newDistributeSvc(q, ranker, &stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 1, 1); err == nil {
		t.Fatal("expected conflict for ineligible group, got nil")
	}
}

func TestAdminGroupService_DistributePrizes_HappyPath_CreditApplied(t *testing.T) {
	q := &stubQuinielaRepo{quiniela: &domain.Quiniela{ID: 1, EntryFee: 10_000}}
	ranker := &noopRanker{result: eligibleLeaderboard(5, 7)}
	svc := newDistributeSvc(q, ranker, &stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 1, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// KYC escrow path: Tier 0 user wins → freeze path taken inside atomic tx.
func TestAdminGroupService_DistributePrizes_Tier0Winner_PrizeFrozenInEscrow(t *testing.T) {
	q := &stubQuinielaRepo{quiniela: &domain.Quiniela{ID: 2, EntryFee: 10_000}}
	ranker := &noopRanker{result: eligibleLeaderboardTier0(42)}
	crediter := &stubPrizeCrediter{credited: false} // notification call for freeze winner
	svc := newDistributeSvc(q, ranker, crediter)
	if err := svc.DistributePrizes(context.Background(), 2, 99); err != nil {
		t.Fatalf("unexpected error: prizes should succeed even when frozen via KYC escrow: %v", err)
	}
}

func TestAdminGroupService_DistributePrizes_AtomicRepoError_Propagates(t *testing.T) {
	q := &stubQuinielaRepo{
		quiniela:      &domain.Quiniela{ID: 1, EntryFee: 10_000},
		distributeErr: errors.New("db error"),
	}
	ranker := &noopRanker{result: eligibleLeaderboard(5)}
	svc := newDistributeSvc(q, ranker, &stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 1, 99); err == nil {
		t.Fatal("expected atomic repo error to propagate, got nil")
	}
}

// TestAdminGroupService_DistributePrizes_Idempotency_SecondCallReturnsConflict verifies
// that a second DistributePrizes call returns apperrors.Conflict (HTTP 409).
func TestAdminGroupService_DistributePrizes_Idempotency_SecondCallReturnsConflict(t *testing.T) {
	q := &stubQuinielaRepo{
		quiniela:              &domain.Quiniela{ID: 3, EntryFee: 5_000},
		distributeAlreadyDone: true,
	}
	ranker := &noopRanker{result: eligibleLeaderboard(10)}
	svc := newDistributeSvc(q, ranker, &stubPrizeCrediter{credited: true})
	err := svc.DistributePrizes(context.Background(), 3, 99)
	if err == nil {
		t.Fatal("expected Conflict on second distribution call, got nil")
	}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %T: %v", err, err)
	}
}

// TestAdminGroupService_DistributePrizes_MixedTiers verifies that Tier2+ winners
// go into direct credits and Tier0 winners go into the freeze slice.
func TestAdminGroupService_DistributePrizes_MixedTiers_CorrectClassification(t *testing.T) {
	var gotCredits []int
	var gotFreezes []int

	qr := &captureDistributeRepo{
		quiniela: &domain.Quiniela{ID: 4, EntryFee: 10_000},
		onDistribute: func(credits []repository.PrizeCredit, freezes []repository.PrizeFreeze) {
			for _, c := range credits {
				gotCredits = append(gotCredits, c.UserID)
			}
			for _, f := range freezes {
				gotFreezes = append(gotFreezes, f.UserID)
			}
		},
	}
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
		{User: &domain.User{ID: 2, KYCTier: domain.KYCTierUnverified}, PrizeWinner: true},
		{User: &domain.User{ID: 3, KYCTier: domain.KYCTierThree}, PrizeWinner: true},
	}
	ranker := &noopRanker{result: &LeaderboardResult{
		Entries:           entries,
		ActivePaidMembers: 3,
		WinnerCount:       3,
		EligibleForPrizes: true,
	}}
	svc := NewAdminGroupService(qr, &stubMemberRepo{}, &noopSnapshotter{}, ranker, &noopAuditLogger{}, zap.NewNop())
	svc.(*adminGroupService).SetPrizeCrediter(&stubPrizeCrediter{credited: false})
	if err := svc.DistributePrizes(context.Background(), 4, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotCredits) != 2 {
		t.Errorf("expected 2 direct credits (IDs 1,3), got %v", gotCredits)
	}
	if len(gotFreezes) != 1 {
		t.Errorf("expected 1 freeze (ID 2), got %v", gotFreezes)
	}
}

// ── Prize pool rounding ───────────────────────────────────────────────────────

// TestAdminGroupService_DistributePrizes_ExactDivision_NoRemainderLost verifies
// that when the pool divides evenly every winner receives the same amount and
// the sum equals the pool exactly.
func TestAdminGroupService_DistributePrizes_ExactDivision_NoRemainderLost(t *testing.T) {
	// Pool = Q100 (10 000 cents), 2 winners → 5 000 cents each, remainder 0.
	var gotAmounts []int
	qr := &captureDistributeRepo{
		quiniela: &domain.Quiniela{ID: 5, EntryFee: 5_000},
		onDistribute: func(credits []repository.PrizeCredit, _ []repository.PrizeFreeze) {
			for _, c := range credits {
				gotAmounts = append(gotAmounts, c.AmountCents)
			}
		},
	}
	ranker := &noopRanker{result: &LeaderboardResult{
		Entries: []*domain.LeaderboardEntry{
			{User: &domain.User{ID: 1, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
			{User: &domain.User{ID: 2, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
		},
		ActivePaidMembers: 2,
		WinnerCount:       2,
		EligibleForPrizes: true,
	}}
	svc := NewAdminGroupService(qr, &stubMemberRepo{}, &noopSnapshotter{}, ranker, &noopAuditLogger{}, zap.NewNop())
	svc.(*adminGroupService).SetPrizeCrediter(&stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 5, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotAmounts) != 2 {
		t.Fatalf("expected 2 credit records, got %d", len(gotAmounts))
	}
	if gotAmounts[0] != 5_000 || gotAmounts[1] != 5_000 {
		t.Errorf("amounts: got %v; want [5000 5000]", gotAmounts)
	}
}

// TestAdminGroupService_DistributePrizes_RemainderCreditedToFirstWinner verifies
// that integer division surplus goes to the first ranked winner so the full
// pool is distributed without silent loss.
func TestAdminGroupService_DistributePrizes_RemainderCreditedToFirstWinner(t *testing.T) {
	// Pool = Q100 (10 000 cents), 3 winners → 3 333 cents each with 1 cent
	// remainder. First winner should receive 3 334; others 3 333.
	// EntryFee=10 000, ActivePaidMembers=1 → pool=10 000, winners=3.
	var gotAmounts []int
	qr := &captureDistributeRepo{
		quiniela: &domain.Quiniela{ID: 6, EntryFee: 10_000},
		onDistribute: func(credits []repository.PrizeCredit, _ []repository.PrizeFreeze) {
			for _, c := range credits {
				gotAmounts = append(gotAmounts, c.AmountCents)
			}
		},
	}
	ranker := &noopRanker{result: &LeaderboardResult{
		Entries: []*domain.LeaderboardEntry{
			{User: &domain.User{ID: 1, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
			{User: &domain.User{ID: 2, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
			{User: &domain.User{ID: 3, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
		},
		ActivePaidMembers: 1, // pool = 10 000 * 1 = 10 000
		WinnerCount:       3,
		EligibleForPrizes: true,
	}}
	svc := NewAdminGroupService(qr, &stubMemberRepo{}, &noopSnapshotter{}, ranker, &noopAuditLogger{}, zap.NewNop())
	svc.(*adminGroupService).SetPrizeCrediter(&stubPrizeCrediter{credited: true})
	if err := svc.DistributePrizes(context.Background(), 6, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotAmounts) != 3 {
		t.Fatalf("expected 3 credit records, got %d", len(gotAmounts))
	}
	total := 0
	for _, a := range gotAmounts {
		total += a
	}
	if total != 10_000 {
		t.Errorf("total credited %d cents; want 10 000 — remainder was lost", total)
	}
	// First winner receives the remainder.
	if gotAmounts[0] != 3_334 {
		t.Errorf("first winner: got %d; want 3 334 (3 333 + 1 remainder)", gotAmounts[0])
	}
	if gotAmounts[1] != 3_333 || gotAmounts[2] != 3_333 {
		t.Errorf("other winners: got %v; want [3333 3333]", gotAmounts[1:])
	}
}

// TestAdminGroupService_DistributePrizes_RemainderWithKYCFreeze verifies that
// remainder is correctly applied when the first winner is a KYC-freeze user —
// their freeze amount must include the remainder, not just prizePerWinner.
func TestAdminGroupService_DistributePrizes_RemainderWithKYCFreeze(t *testing.T) {
	// Pool = 10 000, 3 winners → prizePerWinner=3 333, remainder=1.
	// First winner is Tier0 → freeze path. Freeze amount must be 3 334.
	var gotFreezeAmounts []int
	var gotCreditAmounts []int
	qr := &captureDistributeRepo{
		quiniela: &domain.Quiniela{ID: 7, EntryFee: 10_000},
		onDistribute: func(credits []repository.PrizeCredit, freezes []repository.PrizeFreeze) {
			for _, c := range credits {
				gotCreditAmounts = append(gotCreditAmounts, c.AmountCents)
			}
			for _, f := range freezes {
				gotFreezeAmounts = append(gotFreezeAmounts, f.AmountCents)
			}
		},
	}
	ranker := &noopRanker{result: &LeaderboardResult{
		Entries: []*domain.LeaderboardEntry{
			{User: &domain.User{ID: 1, KYCTier: domain.KYCTierUnverified}, PrizeWinner: true}, // gets remainder
			{User: &domain.User{ID: 2, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
			{User: &domain.User{ID: 3, KYCTier: domain.KYCTierTwo}, PrizeWinner: true},
		},
		ActivePaidMembers: 1,
		WinnerCount:       3,
		EligibleForPrizes: true,
	}}
	svc := NewAdminGroupService(qr, &stubMemberRepo{}, &noopSnapshotter{}, ranker, &noopAuditLogger{}, zap.NewNop())
	svc.(*adminGroupService).SetPrizeCrediter(&stubPrizeCrediter{credited: false})
	if err := svc.DistributePrizes(context.Background(), 7, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotFreezeAmounts) != 1 || gotFreezeAmounts[0] != 3_334 {
		t.Errorf("freeze amount: got %v; want [3334]", gotFreezeAmounts)
	}
	if len(gotCreditAmounts) != 2 {
		t.Fatalf("expected 2 direct credits, got %d", len(gotCreditAmounts))
	}
	total := gotFreezeAmounts[0] + gotCreditAmounts[0] + gotCreditAmounts[1]
	if total != 10_000 {
		t.Errorf("total distributed %d cents; want 10 000", total)
	}
}

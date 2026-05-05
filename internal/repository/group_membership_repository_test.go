package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── GroupMembershipRepository ─────────────────────────────────────────────────

func TestGroupMembershipRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()
	m := &domain.GroupMembership{
		QuinielaID: q.ID,
		UserID:     u.ID,
		Status:     domain.MembershipActive,
		Paid:       true,
		JoinedAt:   &now,
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if !m.Paid {
		t.Error("expected Paid = true after hydration")
	}
}

func TestGroupMembershipRepository_Create_FreeMembership_PaidFalse(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	m := &domain.GroupMembership{
		QuinielaID: q.ID,
		UserID:     u.ID,
		Status:     domain.MembershipPending,
		Paid:       false,
	}

	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Paid {
		t.Error("expected Paid = false")
	}
	if m.JoinedAt != nil {
		t.Error("expected JoinedAt = nil for pending membership")
	}
}

func TestGroupMembershipRepository_Create_ExceedsMaxMembers_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	maxMembers := 1
	quinielaRepo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{
		Name:           "Capped " + nextCode(),
		OwnerID:        owner.ID,
		InviteCode:     nextCode(),
		Currency:       defaultCurrency,
		PrizeThreshold: domain.DefaultPrizeThreshold,
		MaxMembers:     &maxMembers,
	}
	if err := quinielaRepo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}

	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()

	m1 := &domain.GroupMembership{QuinielaID: q.ID, UserID: owner.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
	if err := repo.Create(context.Background(), m1); err != nil {
		t.Fatalf("first membership: %v", err)
	}

	u2 := seedUser(t)
	m2 := &domain.GroupMembership{QuinielaID: q.ID, UserID: u2.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
	if err := repo.Create(context.Background(), m2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict when exceeding max_members, got %v", err)
	}
}

func TestGroupMembershipRepository_Update_ExceedsMaxMembers_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	maxMembers := 1
	quinielaRepo := repository.NewPostgresQuinielaRepository(testDB)
	q := &domain.Quiniela{
		Name:           "Capped " + nextCode(),
		OwnerID:        owner.ID,
		InviteCode:     nextCode(),
		Currency:       defaultCurrency,
		PrizeThreshold: domain.DefaultPrizeThreshold,
		MaxMembers:     &maxMembers,
	}
	if err := quinielaRepo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}

	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()

	m1 := &domain.GroupMembership{QuinielaID: q.ID, UserID: owner.ID, Status: domain.MembershipActive, Paid: true, JoinedAt: &now}
	if err := repo.Create(context.Background(), m1); err != nil {
		t.Fatalf("first membership: %v", err)
	}

	u2 := seedUser(t)
	m2 := &domain.GroupMembership{QuinielaID: q.ID, UserID: u2.ID, Status: domain.MembershipPending, Paid: false}
	if err := repo.Create(context.Background(), m2); err != nil {
		t.Fatalf("pending membership: %v", err)
	}
	m2.Status = domain.MembershipActive
	m2.JoinedAt = &now
	if err := repo.Update(context.Background(), m2); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict when approving past max_members, got %v", err)
	}
}

func TestGroupMembershipRepository_GetByQuinielaAndUser_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	created := seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByQuinielaAndUser(context.Background(), q.ID, u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected membership, got nil")
	}
	if got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf(repoMsgStatusActive, got.Status)
	}
}

func TestGroupMembershipRepository_GetByQuinielaAndUser_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByQuinielaAndUser(context.Background(), 99999, 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestGroupMembershipRepository_Update_ChangesStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	m := seedMembership(t, q.ID, u.ID, domain.MembershipPending, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	now := time.Now().UTC()
	m.Status = domain.MembershipActive
	m.Paid = true
	m.JoinedAt = &now
	if err := repo.Update(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("status not updated: got %q", m.Status)
	}
	if !m.Paid {
		t.Error("paid not updated to true")
	}
}

func TestGroupMembershipRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	ghost := &domain.GroupMembership{ID: 99999, Status: domain.MembershipLeft}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestGroupMembershipRepository_MarkPaid_SetsPaidTrue(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedMembership(t, q.ID, u.ID, domain.MembershipActive, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.MarkPaid(context.Background(), q.ID, u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !got.Paid {
		t.Error("expected Paid = true after MarkPaid")
	}
}

func TestGroupMembershipRepository_MarkPaid_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	if _, err := repo.MarkPaid(context.Background(), 99999, 99999); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestGroupMembershipRepository_ListByQuiniela_ReturnsAllMembers(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, member.ID, domain.MembershipActive, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	members, err := repo.ListByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

func TestGroupMembershipRepository_ListByQuiniela_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	members, err := repo.ListByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

func TestGroupMembershipRepository_ListByUser_ReturnsGroups(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	seedMembership(t, q1.ID, u.ID, domain.MembershipActive, true)
	seedMembership(t, q2.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	groups, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestGroupMembershipRepository_ListByUser_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	groups, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestGroupMembershipRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	created := seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected membership, got nil")
	}
	if got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf(repoMsgStatusActive, got.Status)
	}
}

func TestGroupMembershipRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestGroupMembershipRepository_RequestJoinByInviteCode_NewMembershipCreatesPendingRow(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	joiner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	gotQ, gotM, err := repo.RequestJoinByInviteCode(context.Background(), q.InviteCode, joiner.ID)
	if err != nil {
		t.Fatalf("RequestJoinByInviteCode: %v", err)
	}
	if gotQ == nil || gotQ.ID != q.ID {
		t.Fatalf("expected quiniela %d, got %+v", q.ID, gotQ)
	}
	if gotM == nil {
		t.Fatal("expected membership, got nil")
	}
	if gotM.Status != domain.MembershipPending {
		t.Errorf("expected pending membership, got %q", gotM.Status)
	}
	if gotM.UserID != joiner.ID {
		t.Errorf("expected user %d, got %d", joiner.ID, gotM.UserID)
	}
}

func TestGroupMembershipRepository_RequestJoinByInviteCode_LeftMembershipBecomesPending(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	joiner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	existing := seedMembership(t, q.ID, joiner.ID, domain.MembershipLeft, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	gotQ, gotM, err := repo.RequestJoinByInviteCode(context.Background(), q.InviteCode, joiner.ID)
	if err != nil {
		t.Fatalf("RequestJoinByInviteCode: %v", err)
	}
	if gotQ == nil || gotQ.ID != q.ID {
		t.Fatalf("expected quiniela %d, got %+v", q.ID, gotQ)
	}
	if gotM.ID != existing.ID {
		t.Errorf("expected membership id %d, got %d", existing.ID, gotM.ID)
	}
	if gotM.Status != domain.MembershipPending {
		t.Errorf("expected pending status, got %q", gotM.Status)
	}
	if gotM.JoinedAt != nil {
		t.Error("expected JoinedAt=nil after rejoin request")
	}
}

func TestGroupMembershipRepository_CountActive_ReturnsCount(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, member.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	count, err := repo.CountActive(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if count != 2 {
		t.Errorf("expected 2 active members, got %d", count)
	}
}

func TestGroupMembershipRepository_CountActive_IgnoresPendingAndLeft(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	u3 := seedUser(t)
	q := seedQuiniela(t, u1.ID)
	seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	seedMembership(t, q.ID, u2.ID, domain.MembershipPending, false)
	seedMembership(t, q.ID, u3.ID, domain.MembershipLeft, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	count, err := repo.CountActive(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if count != 1 {
		t.Errorf("expected 1 active member, got %d", count)
	}
}

// ── GroupMembershipRepository - OldestActiveMember ────────────────────────────

func TestGroupMembershipRepository_OldestActiveMember_ReturnsMemberWithEarliestJoinedAt(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m1 := seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	time.Sleep(2 * time.Millisecond)
	_ = seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.OldestActiveMember(context.Background(), q.ID, owner.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected a membership, got nil")
	}
	if got.ID != m1.ID {
		t.Errorf("expected oldest member ID %d, got %d", m1.ID, got.ID)
	}
}

func TestGroupMembershipRepository_OldestActiveMember_ExcludesSpecifiedUser(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	_ = seedMembership(t, q.ID, u1.ID, domain.MembershipActive, true)
	time.Sleep(2 * time.Millisecond)
	m2 := seedMembership(t, q.ID, u2.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.OldestActiveMember(context.Background(), q.ID, u1.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected a membership, got nil")
	}
	if got.ID != m2.ID {
		t.Errorf("expected member ID %d (excluded u1), got %d", m2.ID, got.ID)
	}
}

func TestGroupMembershipRepository_OldestActiveMember_NoSuccessor_ReturnsNil(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	got, err := repo.OldestActiveMember(context.Background(), q.ID, owner.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil when no successor, got membership ID %d", got.ID)
	}
}

// ── GroupMembershipRepository - SetRole ────────────────────────────────────────

func TestGroupMembershipRepository_SetRole_UpdatesRole(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	u := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m := seedMembership(t, q.ID, u.ID, domain.MembershipActive, true)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	if err := repo.SetRole(context.Background(), m.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	updated, err := repo.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("expected role %q, got %q", domain.MembershipRoleCreateOwner, updated.Role)
	}
}

func TestGroupMembershipRepository_SetRole_NotFound_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	err := repo.SetRole(context.Background(), 999999, domain.MembershipRoleCreateOwner)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing membership, got %v", err)
	}
}

// ── GroupMembershipRepository extension (RemoveByAdmin) ──────────────────────

func TestGroupMembershipRepository_RemoveByAdmin_SetsStatusLeft(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	admin := seedUser(t)
	m := seedActiveMembership(t, q.ID, member.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	if err := repo.RemoveByAdmin(context.Background(), m.ID, admin.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), m.ID)
	if got.Status != domain.MembershipLeft {
		t.Errorf("expected status %q, got %q", domain.MembershipLeft, got.Status)
	}
}

func TestGroupMembershipRepository_RemoveByAdmin_NotFoundWhenInactive(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	admin := seedUser(t)
	m := seedActiveMembership(t, q.ID, member.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	_ = repo.RemoveByAdmin(context.Background(), m.ID, admin.ID)
	err := repo.RemoveByAdmin(context.Background(), m.ID, admin.ID)
	if !isNotFound(err) {
		t.Errorf("expected not-found on second remove, got %v", err)
	}
}

// ── GroupMembershipRepository admin extensions ────────────────────────────────

func TestGroupMembershipRepository_ListGroupIDsWithoutOwner_IncludesOrphan(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ids, err := repo.ListGroupIDsWithoutOwner(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	found := false
	for _, id := range ids {
		if id == q.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected quiniela %d to appear in orphan list, got %v", q.ID, ids)
	}
}

func TestGroupMembershipRepository_ListGroupIDsWithoutOwner_ExcludesWithOwner(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	now := time.Now().UTC()
	m := &domain.GroupMembership{
		QuinielaID: q.ID,
		UserID:     owner.ID,
		Status:     domain.MembershipActive,
		Role:       domain.MembershipRoleCreateOwner,
		JoinedAt:   &now,
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}

	ids, err := repo.ListGroupIDsWithoutOwner(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	for _, id := range ids {
		if id == q.ID {
			t.Errorf("quiniela %d with active owner should not appear in orphan list", q.ID)
		}
	}
}

func TestGroupMembershipRepository_ListStalePending_ReturnsPending(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedMembership(t, q.ID, member.ID, domain.MembershipPending, false)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	stale, err := repo.ListStalePending(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale pending, got %d", len(stale))
	}
}

func TestGroupMembershipRepository_ListStalePending_ExcludesActive(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedActiveMembership(t, q.ID, member.ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	stale, err := repo.ListStalePending(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale (active member filtered out), got %d", len(stale))
	}
}

// ── GroupMembershipRepository.BulkRemoveByAdmin ───────────────────────────────

func TestGroupMembershipRepository_BulkRemoveByAdmin_RemovesAllIDs(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m1 := seedActiveMembership(t, q.ID, seedUser(t).ID)
	m2 := seedActiveMembership(t, q.ID, seedUser(t).ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	succeeded, err := repo.BulkRemoveByAdmin(context.Background(), q.ID, []int{m1.ID, m2.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(succeeded))
	}

	for _, id := range []int{m1.ID, m2.ID} {
		got, _ := repo.GetByID(context.Background(), id)
		if got == nil || got.Status != domain.MembershipLeft {
			t.Errorf("membership %d: expected status left, got %v", id, got)
		}
	}
}

func TestGroupMembershipRepository_BulkRemoveByAdmin_InactiveSkipped(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	m := seedActiveMembership(t, q.ID, seedUser(t).ID)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	_, _ = repo.BulkRemoveByAdmin(context.Background(), q.ID, []int{m.ID}, admin.ID)

	succeeded, err := repo.BulkRemoveByAdmin(context.Background(), q.ID, []int{m.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 0 {
		t.Errorf("expected 0 succeeded for already-inactive membership, got %d", len(succeeded))
	}
}

func TestGroupMembershipRepository_BulkRemoveByAdmin_CancelledContext_ReturnsError(t *testing.T) {
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.BulkRemoveByAdmin(ctx, 1, []int{1}, 1)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ── GroupMembershipRepository.TransferOwnershipRoles ─────────────────────────

func TestGroupMembershipRepository_TransferOwnershipRoles_HappyPath_SwapsRoles(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	member := seedUser(t)
	q := seedQuiniela(t, owner.ID)

	ownerMembership := seedMembership(t, q.ID, owner.ID, domain.MembershipActive, false)
	if err := repo.SetRole(context.Background(), ownerMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf("seed owner role: %v", err)
	}
	memberMembership := seedActiveMembership(t, q.ID, member.ID)

	if err := repo.TransferOwnershipRoles(context.Background(), q.ID, memberMembership.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	demoted, err := repo.GetByID(context.Background(), ownerMembership.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if demoted.Role != domain.MembershipRoleMember {
		t.Errorf("expected old owner role=member after transfer, got %q", demoted.Role)
	}

	promoted, err := repo.GetByID(context.Background(), memberMembership.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if promoted.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("expected new owner role=owner after transfer, got %q", promoted.Role)
	}
}

func TestGroupMembershipRepository_TransferOwnershipRoles_InvalidPromotee_RollsBackDemotion(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)

	ownerMembership := seedMembership(t, q.ID, owner.ID, domain.MembershipActive, false)
	if err := repo.SetRole(context.Background(), ownerMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf("seed owner role: %v", err)
	}

	// Promote a non-existent membership - the transaction must roll back.
	if err := repo.TransferOwnershipRoles(context.Background(), q.ID, 999999); err == nil {
		t.Fatal("expected error for non-existent promotee membership, got nil")
	}

	unchanged, err := repo.GetByID(context.Background(), ownerMembership.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if unchanged.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("rollback failed: expected role=owner after aborted transfer, got %q", unchanged.Role)
	}
}

func TestGroupMembershipRepository_TransferOwnershipRoles_CancelledContext_ReturnsError(t *testing.T) {
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.TransferOwnershipRoles(ctx, 1, 1); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ── GroupMembershipRepository.ApproveMembership ───────────────────────────────

func TestGroupMembershipRepository_ApproveMembership_PromotesToActiveAndSyncsStatus(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	qRepo := repository.NewPostgresQuinielaRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedActiveMembership(t, q.ID, owner.ID)
	seedActiveMembership(t, q.ID, seedUser(t).ID)

	joiner := seedUser(t)
	pending := seedMembership(t, q.ID, joiner.ID, domain.MembershipPending, false)

	now := time.Now().UTC()
	m, err := repo.ApproveMembership(context.Background(), pending.ID, q.ID, now, domain.MinMembersForActive)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("expected status=active after approval, got %q", m.Status)
	}
	if m.JoinedAt == nil {
		t.Error("expected JoinedAt to be set after approval")
	}

	q2, err := qRepo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q2.Status != domain.QuinielaStatusActive {
		t.Errorf("expected quiniela status=active after approval, got %q", q2.Status)
	}
}

func TestGroupMembershipRepository_ApproveMembership_BelowThreshold_QuinielaStaysInactive(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	qRepo := repository.NewPostgresQuinielaRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedActiveMembership(t, q.ID, owner.ID)

	joiner := seedUser(t)
	pending := seedMembership(t, q.ID, joiner.ID, domain.MembershipPending, false)

	_, err := repo.ApproveMembership(context.Background(), pending.ID, q.ID, time.Now().UTC(), domain.MinMembersForActive)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	q2, err := qRepo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q2.Status != domain.QuinielaStatusInactive {
		t.Errorf("expected quiniela status=inactive with only 2 members, got %q", q2.Status)
	}
}

func TestGroupMembershipRepository_ApproveMembership_AlreadyApproved_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	active := seedActiveMembership(t, q.ID, owner.ID)

	err := func() error {
		_, e := repo.ApproveMembership(context.Background(), active.ID, q.ID, time.Now().UTC(), domain.MinMembersForActive)
		return e
	}()
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for already-active membership, got %v", err)
	}
}

func TestGroupMembershipRepository_ApproveMembership_ExceedsMaxMembers_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	qRepo := repository.NewPostgresQuinielaRepository(testDB)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	maxMembers := 1
	q := &domain.Quiniela{
		Name:           "Capped " + nextCode(),
		OwnerID:        owner.ID,
		InviteCode:     nextCode(),
		Currency:       defaultCurrency,
		PrizeThreshold: domain.DefaultPrizeThreshold,
		MaxMembers:     &maxMembers,
	}
	if err := qRepo.Create(context.Background(), q); err != nil {
		t.Fatalf("seed quiniela: %v", err)
	}

	seedActiveMembership(t, q.ID, owner.ID)

	joiner := seedUser(t)
	pending := seedMembership(t, q.ID, joiner.ID, domain.MembershipPending, false)

	_, err := repo.ApproveMembership(context.Background(), pending.ID, q.ID, time.Now().UTC(), domain.MinMembersForActive)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict when approving past max_members, got %v", err)
	}
}

func TestGroupMembershipRepository_ApproveMembership_CancelledContext_ReturnsError(t *testing.T) {
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.ApproveMembership(ctx, 1, 1, time.Now().UTC(), domain.MinMembersForActive); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ── GroupMembershipRepository.LeaveMembership ─────────────────────────────────

func TestGroupMembershipRepository_LeaveMembership_SetsLeftAndSyncsStatus(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	qRepo := repository.NewPostgresQuinielaRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedActiveMembership(t, q.ID, owner.ID)

	u2, u3 := seedUser(t), seedUser(t)
	seedActiveMembership(t, q.ID, u2.ID)
	seedActiveMembership(t, q.ID, u3.ID)

	if err := qRepo.UpdateStatus(context.Background(), q.ID, domain.QuinielaStatusActive); err != nil {
		t.Fatalf("seed quiniela status: %v", err)
	}

	if err := repo.LeaveMembership(context.Background(), q.ID, u3.ID, time.Now().UTC(), domain.MinMembersForActive); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	m, err := repo.GetByQuinielaAndUser(context.Background(), q.ID, u3.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if m.Status != domain.MembershipLeft {
		t.Errorf("expected status=left after leave, got %q", m.Status)
	}
	if m.JoinedAt != nil {
		t.Error("expected JoinedAt=nil after leave")
	}
	if m.RemovedAt == nil {
		t.Error("expected RemovedAt to be set after leave")
	}
	if m.RemovedBy != nil {
		t.Error("expected RemovedBy=nil for self-exit")
	}

	q2, err := qRepo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q2.Status != domain.QuinielaStatusInactive {
		t.Errorf("expected quiniela status=inactive after member leaves, got %q", q2.Status)
	}
}

func TestGroupMembershipRepository_LeaveMembership_StaysActive_WhenEnoughMembersRemain(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	qRepo := repository.NewPostgresQuinielaRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	seedActiveMembership(t, q.ID, owner.ID)

	users := make([]*domain.User, 3)
	for i := range users {
		users[i] = seedUser(t)
		seedActiveMembership(t, q.ID, users[i].ID)
	}
	if err := qRepo.UpdateStatus(context.Background(), q.ID, domain.QuinielaStatusActive); err != nil {
		t.Fatalf("seed quiniela status: %v", err)
	}

	if err := repo.LeaveMembership(context.Background(), q.ID, users[0].ID, time.Now().UTC(), domain.MinMembersForActive); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	q2, err := qRepo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q2.Status != domain.QuinielaStatusActive {
		t.Errorf("expected quiniela status=active with 3 remaining members, got %q", q2.Status)
	}
}

func TestGroupMembershipRepository_LeaveMembership_NotActive_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	pending := seedMembership(t, q.ID, owner.ID, domain.MembershipPending, false)

	err := repo.LeaveMembership(context.Background(), q.ID, pending.UserID, time.Now().UTC(), domain.MinMembersForActive)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for non-active member leave, got %v", err)
	}
}

func TestGroupMembershipRepository_LeaveMembershipAndTransferOwnership_SwapsOwnerAndLeavesAtomically(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)
	qRepo := repository.NewPostgresQuinielaRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	ownerMembership := seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	if err := repo.SetRole(context.Background(), ownerMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf("seed owner role: %v", err)
	}

	successorUser := seedUser(t)
	successorMembership := seedMembership(t, q.ID, successorUser.ID, domain.MembershipActive, true)

	if err := qRepo.UpdateStatus(context.Background(), q.ID, domain.QuinielaStatusActive); err != nil {
		t.Fatalf("seed quiniela status: %v", err)
	}

	if err := repo.LeaveMembershipAndTransferOwnership(
		context.Background(),
		q.ID,
		owner.ID,
		successorMembership.ID,
		time.Now().UTC(),
		domain.MinMembersForActive,
	); err != nil {
		t.Fatalf("LeaveMembershipAndTransferOwnership: %v", err)
	}

	leftOwner, err := repo.GetByQuinielaAndUser(context.Background(), q.ID, owner.ID)
	if err != nil {
		t.Fatalf("reload leaving owner: %v", err)
	}
	if leftOwner.Status != domain.MembershipLeft {
		t.Errorf("expected leaving owner status=left, got %q", leftOwner.Status)
	}
	if leftOwner.Role != domain.MembershipRoleMember {
		t.Errorf("expected leaving owner role=member after exit, got %q", leftOwner.Role)
	}

	promoted, err := repo.GetByID(context.Background(), successorMembership.ID)
	if err != nil {
		t.Fatalf("reload successor: %v", err)
	}
	if promoted.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("expected successor role=%q, got %q", domain.MembershipRoleCreateOwner, promoted.Role)
	}

	q2, err := qRepo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf("reload quiniela: %v", err)
	}
	if q2.Status != domain.QuinielaStatusInactive {
		t.Errorf("expected quiniela status=inactive with one active member remaining, got %q", q2.Status)
	}
}

func TestGroupMembershipRepository_LeaveMembershipAndTransferOwnership_InvalidSuccessor_RollsBackLeave(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	ownerMembership := seedMembership(t, q.ID, owner.ID, domain.MembershipActive, true)
	if err := repo.SetRole(context.Background(), ownerMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
		t.Fatalf("seed owner role: %v", err)
	}

	member := seedUser(t)
	seedMembership(t, q.ID, member.ID, domain.MembershipActive, true)

	if err := repo.LeaveMembershipAndTransferOwnership(
		context.Background(),
		q.ID,
		owner.ID,
		999999,
		time.Now().UTC(),
		domain.MinMembersForActive,
	); err == nil {
		t.Fatal("expected error for invalid successor membership, got nil")
	}

	unchangedOwner, err := repo.GetByQuinielaAndUser(context.Background(), q.ID, owner.ID)
	if err != nil {
		t.Fatalf("reload owner after rollback: %v", err)
	}
	if unchangedOwner.Status != domain.MembershipActive {
		t.Errorf("expected owner to remain active after rollback, got %q", unchangedOwner.Status)
	}
	if unchangedOwner.Role != domain.MembershipRoleCreateOwner {
		t.Errorf("expected owner role to remain %q after rollback, got %q", domain.MembershipRoleCreateOwner, unchangedOwner.Role)
	}
}

func TestGroupMembershipRepository_LeaveMembershipAndTransferOwnership_CancelledContext_ReturnsError(t *testing.T) {
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.LeaveMembershipAndTransferOwnership(ctx, 1, 1, 2, time.Now().UTC(), domain.MinMembersForActive); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestGroupMembershipRepository_LeaveMembership_CancelledContext_ReturnsError(t *testing.T) {
	repo := repository.NewPostgresGroupMembershipRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.LeaveMembership(ctx, 1, 1, time.Now().UTC(), domain.MinMembersForActive); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

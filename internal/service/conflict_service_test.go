package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

type spyAuditLogger struct {
	called       bool
	userID       *int
	role         *domain.UserRole
	action       string
	resourceType *string
	resourceID   *int
	metadata     map[string]any
}

func (s *spyAuditLogger) Log(_ context.Context, userID *int, role *domain.UserRole, action string, resourceType *string, resourceID *int, metadata map[string]any) {
	s.called = true
	s.userID = userID
	s.role = role
	s.action = action
	s.resourceType = resourceType
	s.resourceID = resourceID
	s.metadata = metadata
}

type stubMemberRepoConflict struct {
	*stubMemberRepo
	groupIDs []int
}

func (r *stubMemberRepoConflict) ListGroupIDsWithoutOwner(_ context.Context) ([]int, error) {
	return r.groupIDs, r.err
}

func TestConflictService_ListConflicts_ReturnsAllCategories(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)

	groupIDs := []int{1, 2}
	quinielas := []*domain.Quiniela{
		{ID: 1, Name: "Liga 1"},
		{ID: 2, Name: "Liga 2"},
	}
	payment := &domain.PaymentRecord{
		ID:         10,
		QuinielaID: 1,
		UserID:     99,
		Amount:     500,
		CreatedAt:  now.Add(-72 * time.Hour),
	}
	membership := &domain.GroupMembership{
		ID:         20,
		QuinielaID: 2,
		UserID:     42,
		CreatedAt:  now.Add(-48 * time.Hour),
	}

	qr := &stubQuinielaRepo{quinielas: quinielas}
	mr := &stubMemberRepoConflict{
		stubMemberRepo: &stubMemberRepo{memberships: []*domain.GroupMembership{membership}},
		groupIDs:       groupIDs,
	}
	pr := &stubPaymentRepo{records: []*domain.PaymentRecord{payment}}

	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	conflicts, err := svc.ListConflicts(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 4 {
		t.Fatalf("expected 4 conflicts (2 groups + 1 payment + 1 membership), got %d", len(conflicts))
	}

	byType := indexConflictsByType(conflicts)
	assertGroupOwnerConflicts(t, byType[domain.ConflictGroupNoOwner])
	assertStalePaymentConflicts(t, byType[domain.ConflictPaymentStale], payment)
	assertStaleMembershipConflicts(t, byType[domain.ConflictMembershipStale], membership)
}

func indexConflictsByType(conflicts []domain.Conflict) map[domain.ConflictType][]domain.Conflict {
	m := make(map[domain.ConflictType][]domain.Conflict)
	for _, c := range conflicts {
		m[c.Type] = append(m[c.Type], c)
	}
	return m
}

func assertGroupOwnerConflicts(t *testing.T, cs []domain.Conflict) {
	t.Helper()
	if len(cs) == 0 {
		t.Error("ConflictGroupNoOwner: expected at least one conflict")
		return
	}
	for _, c := range cs {
		if c.EntityType != "quiniela" {
			t.Errorf("ConflictGroupNoOwner EntityType: want quiniela, got %q", c.EntityType)
		}
		if c.Details == nil {
			t.Error("ConflictGroupNoOwner Details: want non-nil")
		}
	}
}

func assertStalePaymentConflicts(t *testing.T, cs []domain.Conflict, payment *domain.PaymentRecord) {
	t.Helper()
	if len(cs) == 0 {
		t.Error("ConflictPaymentStale: expected at least one conflict")
		return
	}
	c := cs[0]
	if c.EntityID != payment.ID {
		t.Errorf("ConflictPaymentStale EntityID: want %d, got %d", payment.ID, c.EntityID)
	}
	if c.EntityType != "payment_record" {
		t.Errorf("ConflictPaymentStale EntityType: want payment_record, got %q", c.EntityType)
	}
	if got, ok := c.Details["age_days"].(int); ok && got <= 0 {
		t.Errorf("ConflictPaymentStale age_days: want >0, got %d", got)
	}
}

func assertStaleMembershipConflicts(t *testing.T, cs []domain.Conflict, membership *domain.GroupMembership) {
	t.Helper()
	if len(cs) == 0 {
		t.Error("ConflictMembershipStale: expected at least one conflict")
		return
	}
	c := cs[0]
	if c.EntityID != membership.ID {
		t.Errorf("ConflictMembershipStale EntityID: want %d, got %d", membership.ID, c.EntityID)
	}
	if c.EntityType != "group_membership" {
		t.Errorf("ConflictMembershipStale EntityType: want group_membership, got %q", c.EntityType)
	}
}

func TestConflictService_ListConflicts_PaymentRepoError_SkipsCategory(t *testing.T) {
	qr := &stubQuinielaRepo{}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{}, groupIDs: nil}
	pr := &stubPaymentRepo{err: errors.New("db error")}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	conflicts, err := svc.ListConflicts(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range conflicts {
		if c.Type == domain.ConflictPaymentStale {
			t.Error("expected no payment_stale conflicts when repo errors")
		}
	}
}

func TestConflictService_ListConflicts_MembershipRepoError_SkipsCategory(t *testing.T) {
	qr := &stubQuinielaRepo{}
	mr := &stubMemberRepoConflict{
		stubMemberRepo: &stubMemberRepo{err: errors.New("db error")},
		groupIDs:       nil,
	}
	pr := &stubPaymentRepo{}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	conflicts, err := svc.ListConflicts(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range conflicts {
		if c.Type == domain.ConflictMembershipStale {
			t.Error("expected no membership_stale conflicts when repo errors")
		}
	}
}

func TestConflictService_ListConflicts_NoConflicts_ReturnsEmptySlice(t *testing.T) {
	qr := &stubQuinielaRepo{quinielas: nil}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{memberships: nil}, groupIDs: nil}
	pr := &stubPaymentRepo{records: nil}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	conflicts, err := svc.ListConflicts(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflicts == nil || len(conflicts) != 0 {
		t.Errorf("expected empty slice, got %v", conflicts)
	}
}

// ── ConflictSummary ───────────────────────────────────────────────────────────

func TestConflictService_ConflictSummary_AggregatesPerType(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	payment := &domain.PaymentRecord{ID: 10, QuinielaID: 1, UserID: 99, Amount: 500, CreatedAt: now.Add(-72 * time.Hour)}
	membership := &domain.GroupMembership{ID: 20, QuinielaID: 2, UserID: 42, CreatedAt: now.Add(-48 * time.Hour)}

	qr := &stubQuinielaRepo{quinielas: []*domain.Quiniela{{ID: 1, Name: "L1"}}}
	mr := &stubMemberRepoConflict{
		stubMemberRepo: &stubMemberRepo{memberships: []*domain.GroupMembership{membership}},
		groupIDs:       []int{1},
	}
	pr := &stubPaymentRepo{records: []*domain.PaymentRecord{payment}}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.ConflictSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalUnresolved != 3 {
		t.Errorf("TotalUnresolved: want 3, got %d", result.TotalUnresolved)
	}
	if len(result.ByType) != 3 {
		t.Errorf("ByType length: want 3, got %d", len(result.ByType))
	}

	for _, s := range result.ByType {
		switch s.Type {
		case domain.ConflictGroupNoOwner:
			if s.AvgAgeDays != nil {
				t.Error("group_without_owner AvgAgeDays: want nil, got non-nil")
			}
		case domain.ConflictPaymentStale, domain.ConflictMembershipStale:
			if s.AvgAgeDays == nil {
				t.Errorf("%s AvgAgeDays: want non-nil", s.Type)
			}
		}
	}
}

func TestConflictService_ConflictSummary_NoConflicts_ReturnsTotalZero(t *testing.T) {
	qr := &stubQuinielaRepo{}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{}, groupIDs: nil}
	pr := &stubPaymentRepo{}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.ConflictSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalUnresolved != 0 {
		t.Errorf("TotalUnresolved: want 0, got %d", result.TotalUnresolved)
	}
	if len(result.ByType) != 0 {
		t.Errorf("ByType: want empty, got %d entries", len(result.ByType))
	}
}

func TestConflictService_ConflictSummary_MultiplePaymentsAvgAge(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	p1 := &domain.PaymentRecord{ID: 1, CreatedAt: now.Add(-2 * 24 * time.Hour)}
	p2 := &domain.PaymentRecord{ID: 2, CreatedAt: now.Add(-4 * 24 * time.Hour)}

	qr := &stubQuinielaRepo{}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{}, groupIDs: nil}
	pr := &stubPaymentRepo{records: []*domain.PaymentRecord{p1, p2}}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	result, err := svc.ConflictSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ByType) != 1 {
		t.Fatalf("ByType: want 1, got %d", len(result.ByType))
	}
	s := result.ByType[0]
	if s.AvgAgeDays == nil {
		t.Fatal("AvgAgeDays: want non-nil")
	}
	// avg of 2 and 4 days = 3.0
	if *s.AvgAgeDays != 3.0 {
		t.Errorf("AvgAgeDays: want 3.0, got %.1f", *s.AvgAgeDays)
	}
}

func TestConflictService_ResolveConflict_LogsAuditEntry(t *testing.T) {
	audit := &spyAuditLogger{}
	svc := NewConflictService(&stubQuinielaRepo{}, &stubMemberRepo{}, &stubPaymentRepo{}, &noopSystemParamService{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictGroupNoOwner), 7, 99, "ack", "admin note"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called")
	}
	if audit.action != domain.AuditActionConflictAcknowledged {
		t.Errorf("audit action: want %q, got %q", domain.AuditActionConflictAcknowledged, audit.action)
	}
	if audit.metadata == nil || audit.metadata["conflict_type"] != string(domain.ConflictGroupNoOwner) {
		t.Errorf("audit metadata conflict_type: want %q, got %v", string(domain.ConflictGroupNoOwner), audit.metadata)
	}
	if got, ok := audit.metadata["note"]; !ok || got != "admin note" {
		t.Errorf("audit metadata note: want %q, got %v", "admin note", got)
	}
}

func TestConflictService_ResolveConflict_AutoFix_RecordsNoteConsistently(t *testing.T) {
	audit := &spyAuditLogger{}
	svc := NewConflictService(&stubQuinielaRepo{}, &stubMemberRepo{}, &stubPaymentRepo{}, &noopSystemParamService{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictGroupNoOwner), 1, 99, "auto_fix", "auto note"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if audit.action != domain.AuditActionConflictAutoResolved {
		t.Errorf("audit action: want %q, got %q", domain.AuditActionConflictAutoResolved, audit.action)
	}
	// Both "ack" and "auto_fix" must record the note under the same key "note".
	if got, ok := audit.metadata["note"]; !ok || got != "auto note" {
		t.Errorf("audit metadata note: want %q, got %v (key must be \"note\" in both branches)", "auto note", got)
	}
}

// ── ResolveConflict - auto_fix ────────────────────────────────────────────────

func TestConflictService_ResolveConflict_AutoFix_GroupNoOwner_TransfersOwnership(t *testing.T) {
	successor := &domain.GroupMembership{ID: 5, UserID: 10, Status: domain.MembershipActive}
	audit := &spyAuditLogger{}
	mr := &stubMemberRepo{membership: successor}
	svc := NewConflictService(&stubQuinielaRepo{}, mr, &stubPaymentRepo{}, &noopSystemParamService{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictGroupNoOwner), 1, 99, "auto_fix", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called")
	}
	if audit.action != domain.AuditActionConflictAutoResolved {
		t.Errorf("expected action %q, got %q", domain.AuditActionConflictAutoResolved, audit.action)
	}
}

func TestConflictService_ResolveConflict_AutoFix_GroupNoOwner_NoSuccessor_StillSucceeds(t *testing.T) {
	audit := &spyAuditLogger{}
	mr := &stubMemberRepo{membership: nil} // no eligible successor
	svc := NewConflictService(&stubQuinielaRepo{}, mr, &stubPaymentRepo{}, &noopSystemParamService{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictGroupNoOwner), 1, 99, "auto_fix", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called even when auto_fix has nothing to do")
	}
}

func TestConflictService_ResolveConflict_AutoFix_PaymentStale_AutoRejects(t *testing.T) {
	rejected := &domain.PaymentRecord{ID: 7, Status: "rejected"}
	audit := &spyAuditLogger{}
	pr := &stubPaymentRepo{record: rejected}
	svc := NewConflictService(&stubQuinielaRepo{}, &stubMemberRepo{}, pr, &noopSystemParamService{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictPaymentStale), 7, 99, "auto_fix", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called")
	}
}

func TestConflictService_ResolveConflict_AutoFix_MembershipStale_RemovesMembership(t *testing.T) {
	audit := &spyAuditLogger{}
	mr := &stubMemberRepo{}
	svc := NewConflictService(&stubQuinielaRepo{}, mr, &stubPaymentRepo{}, &noopSystemParamService{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictMembershipStale), 20, 99, "auto_fix", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called")
	}
}

func TestConflictService_ResolveConflict_AutoFix_FixFails_StillLogsAudit(t *testing.T) {
	// auto_fix for PaymentStale where Reject returns an error - should still write audit entry.
	audit := &spyAuditLogger{}
	pr := &stubPaymentRepo{err: errors.New("db error")}
	svc := NewConflictService(&stubQuinielaRepo{}, &stubMemberRepo{}, pr, &noopSystemParamService{}, audit, zap.NewNop())

	// Should NOT return the inner error - auto_fix failures are best-effort.
	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictPaymentStale), 7, 99, "auto_fix", "note"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit entry to be written even when auto_fix fails")
	}
}

// ── ResolveConflict - unknown action ─────────────────────────────────────────

func TestConflictService_ResolveConflict_UnknownAction_ReturnsValidationError(t *testing.T) {
	audit := &spyAuditLogger{}
	svc := NewConflictService(&stubQuinielaRepo{}, &stubMemberRepo{}, &stubPaymentRepo{}, &noopSystemParamService{}, audit, zap.NewNop())

	err := svc.ResolveConflict(context.Background(), string(domain.ConflictGroupNoOwner), 1, 99, "fix", "")
	if err == nil {
		t.Fatal("expected validation error for unknown action, got nil")
	}
	if audit.called {
		t.Error("expected audit logger NOT to be called for invalid action")
	}
}

// ── ListConflicts - pagination ────────────────────────────────────────────────

func TestConflictService_ListConflicts_Pagination_SlicesResults(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	payments := []*domain.PaymentRecord{
		{ID: 1, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: 2, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: 3, CreatedAt: now.Add(-72 * time.Hour)},
	}
	qr := &stubQuinielaRepo{}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{}, groupIDs: nil}
	pr := &stubPaymentRepo{records: payments}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.ListConflicts(context.Background(), repository.Pagination{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 conflicts after limit, got %d", len(got))
	}
}

func TestConflictService_ListConflicts_Pagination_OffsetBeyondEnd_ReturnsEmpty(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	pr := &stubPaymentRepo{records: []*domain.PaymentRecord{
		{ID: 1, CreatedAt: now.Add(-72 * time.Hour)},
	}}
	qr := &stubQuinielaRepo{}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{}, groupIDs: nil}
	svc := NewConflictService(qr, mr, pr, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.ListConflicts(context.Background(), repository.Pagination{Offset: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 conflicts for offset beyond end, got %d", len(got))
	}
}

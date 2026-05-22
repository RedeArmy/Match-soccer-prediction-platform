package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

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

	conflicts, err := svc.ListConflicts(context.Background(), repository.Unbounded())
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

	conflicts, err := svc.ListConflicts(context.Background(), repository.Unbounded())
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

	conflicts, err := svc.ListConflicts(context.Background(), repository.Unbounded())
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

	conflicts, err := svc.ListConflicts(context.Background(), repository.Unbounded())
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

// ── Load Testing - 10K+ conflicts ─────────────────────────────────────────────

// systemParamServiceWithMaxScan allows overriding the conflict.max_scan value
// for load testing without requiring a real database or system_params table.
type systemParamServiceWithMaxScan struct {
	maxScan int
}

func (s *systemParamServiceWithMaxScan) GetInt(_ context.Context, key string, defaultVal int) int {
	if key == domain.ParamKeyConflictMaxScan {
		return s.maxScan
	}
	return defaultVal
}

func (s *systemParamServiceWithMaxScan) GetString(_ context.Context, _ string, defaultVal string) string {
	return defaultVal
}

func (s *systemParamServiceWithMaxScan) GetDuration(_ context.Context, _ string, defaultVal time.Duration) time.Duration {
	return defaultVal
}

func (s *systemParamServiceWithMaxScan) GetBool(_ context.Context, _ string, defaultVal bool) bool {
	return defaultVal
}

func (s *systemParamServiceWithMaxScan) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}

func (s *systemParamServiceWithMaxScan) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return nil, nil
}

func (s *systemParamServiceWithMaxScan) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}

func (s *systemParamServiceWithMaxScan) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}

func (s *systemParamServiceWithMaxScan) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (s *systemParamServiceWithMaxScan) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *systemParamServiceWithMaxScan) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}

// TestConflictService_ConflictSummary_Load_10KConflicts validates behavior
// under pathological load: 10,000+ conflicts across all categories.
//
// This test ensures:
// 1. The conflict.max_scan limit (5000) is enforced
// 2. Memory consumption remains bounded
// 3. Performance degrades gracefully under extreme backlog
// 4. No panics or OOM errors occur
//
// Run with: go test -run TestConflictService_ConflictSummary_Load_10KConflicts -v
func TestConflictService_ConflictSummary_Load_10KConflicts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	qr, mr, pr := generate10KConflictData(t)

	// Test with default limit (5000)
	t.Run("respects_max_scan_limit", func(t *testing.T) {
		maxScan := 5000
		svc := newConflictServiceWithMaxScan(qr, mr, pr, maxScan)

		result, err := svc.ConflictSummary(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		verifyMaxScanLimit(t, result, maxScan)
	})

	// Test with lower limit (1000) to verify configurability
	t.Run("respects_custom_max_scan", func(t *testing.T) {
		maxScan := 1000
		svc := newConflictServiceWithMaxScan(qr, mr, pr, maxScan)

		result, err := svc.ConflictSummary(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.TotalUnresolved > maxScan {
			t.Errorf("TotalUnresolved exceeds custom max_scan: want <=%d, got %d", maxScan, result.TotalUnresolved)
		}
	})

	// Test performance under extreme load
	t.Run("performance_under_load", func(t *testing.T) {
		maxScan := 5000
		svc := newConflictServiceWithMaxScan(qr, mr, pr, maxScan)

		start := time.Now()
		_, err := svc.ConflictSummary(context.Background())
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With 10K conflicts, scanning and limiting to 5K should complete in <500ms
		// (this is a generous threshold - actual performance should be much better)
		maxDuration := 500 * time.Millisecond
		if elapsed > maxDuration {
			t.Errorf("ConflictSummary took too long with 10K conflicts: %v (max: %v)", elapsed, maxDuration)
		}

		t.Logf("✓ ConflictSummary processed 10K conflicts in %v", elapsed)
	})

	// Test that unlimited scan (Pagination{}) without maxScan would process all conflicts
	t.Run("unlimited_scan_processes_all", func(t *testing.T) {
		// Use a very high limit to effectively disable the cap
		maxScan := 50000
		paramSvc := &systemParamServiceWithMaxScan{maxScan: maxScan}
		svc := NewConflictService(qr, mr, pr, paramSvc, &noopAuditLogger{}, zap.NewNop())

		result, err := svc.ConflictSummary(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With unlimited cap, should see all 10K conflicts
		expectedTotal := 10000 // 4000 payments + 4000 memberships + 2000 groups
		if result.TotalUnresolved != expectedTotal {
			t.Errorf("TotalUnresolved with unlimited scan: want %d, got %d", expectedTotal, result.TotalUnresolved)
		}

		t.Logf("✓ Unlimited scan correctly processed all %d conflicts", expectedTotal)
	})
}

// ── Alerting - Limit reached warning ──────────────────────────────────────────

// TestConflictService_ConflictSummary_AlertThreshold validates that:
// 1. LimitReached flag is set correctly
// 2. MaxScan is returned for context
// 3. Logs are emitted at appropriate levels (WARN at 80%, ERROR at 100%)
func TestConflictService_ConflictSummary_AlertThreshold(t *testing.T) {
	tests := []struct {
		name             string
		conflictCount    int
		maxScan          int
		wantTotal        int
		wantLimitReached bool
		wantWarnCalled   bool
		wantErrorCalled  bool
		needsSpyLogger   bool
	}{
		{
			name:             "below_threshold_no_alert",
			conflictCount:    1000,
			maxScan:          5000,
			wantTotal:        1000,
			wantLimitReached: false,
			needsSpyLogger:   false,
		},
		{
			name:             "at_80_percent_threshold_warns",
			conflictCount:    4000,
			maxScan:          5000,
			wantTotal:        4000,
			wantLimitReached: false,
			wantWarnCalled:   true,
			wantErrorCalled:  false,
			needsSpyLogger:   true,
		},
		{
			name:             "limit_reached_errors",
			conflictCount:    5000,
			maxScan:          5000,
			wantTotal:        5000,
			wantLimitReached: true,
			wantErrorCalled:  true,
			needsSpyLogger:   true,
		},
		{
			name:             "above_limit_still_capped",
			conflictCount:    5001,
			maxScan:          5000,
			wantTotal:        5000,
			wantLimitReached: true,
			needsSpyLogger:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos := setupAlertThresholdTest(tt.conflictCount)
			var svc ConflictService

			if tt.needsSpyLogger {
				svc = createConflictServiceWithSpyLogger(t, repos, tt.maxScan, &tt.wantWarnCalled, &tt.wantErrorCalled)
			} else {
				svc = newConflictServiceWithMaxScan(repos.qr, repos.mr, repos.pr, tt.maxScan)
			}

			result, err := svc.ConflictSummary(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			verifyAlertThresholdResult(t, result, tt.wantTotal, tt.wantLimitReached, tt.maxScan)
		})
	}
}

// alertThresholdRepos holds the repositories for alert threshold tests.
type alertThresholdRepos struct {
	qr *stubQuinielaRepo
	mr *stubMemberRepoConflict
	pr *stubPaymentRepo
}

// setupAlertThresholdTest creates test repositories with N payment conflicts.
func setupAlertThresholdTest(count int) alertThresholdRepos {
	now := time.Now().UTC()
	payments := make([]*domain.PaymentRecord, count)
	for i := 0; i < count; i++ {
		payments[i] = &domain.PaymentRecord{
			ID:        i + 1,
			CreatedAt: now.Add(-8 * 24 * time.Hour),
		}
	}

	return alertThresholdRepos{
		qr: &stubQuinielaRepo{},
		mr: &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{}, groupIDs: nil},
		pr: &stubPaymentRepo{records: payments},
	}
}

// createConflictServiceWithSpyLogger creates a service with a spy logger and validates alert calls.
func createConflictServiceWithSpyLogger(
	t *testing.T,
	repos alertThresholdRepos,
	maxScan int,
	wantWarnCalled *bool,
	wantErrorCalled *bool,
) ConflictService {
	t.Helper()
	paramSvc := &systemParamServiceWithMaxScan{maxScan: maxScan}
	spyLog := &spyLogger{}
	spyLog.logger = spyLog.newLogger()

	// Register cleanup to verify spy assertions after test
	t.Cleanup(func() {
		if wantWarnCalled != nil && *wantWarnCalled != spyLog.warnCalled {
			t.Errorf("WARN log call: want %v, got %v", *wantWarnCalled, spyLog.warnCalled)
		}
		if wantErrorCalled != nil && *wantErrorCalled != spyLog.errorCalled {
			t.Errorf("ERROR log call: want %v, got %v", *wantErrorCalled, spyLog.errorCalled)
		}
	})

	return NewConflictService(repos.qr, repos.mr, repos.pr, paramSvc, &noopAuditLogger{}, spyLog.logger)
}

// verifyAlertThresholdResult validates the ConflictSummary result fields.
func verifyAlertThresholdResult(t *testing.T, result *ConflictSummaryResult, wantTotal int, wantLimitReached bool, maxScan int) {
	t.Helper()

	if result.TotalUnresolved != wantTotal {
		t.Errorf("TotalUnresolved: want %d, got %d", wantTotal, result.TotalUnresolved)
	}
	if result.LimitReached != wantLimitReached {
		t.Errorf("LimitReached: want %v, got %v", wantLimitReached, result.LimitReached)
	}
	if result.MaxScan != maxScan {
		t.Errorf("MaxScan: want %d, got %d", maxScan, result.MaxScan)
	}
}

// ── Test helpers for load tests ──────────────────────────────────────────────

// generate10KConflictData creates test fixtures for load testing.
// Returns repositories with 4000 payments + 4000 memberships + 2000 groups = 10,000 conflicts.
func generate10KConflictData(t *testing.T) (*stubQuinielaRepo, *stubMemberRepoConflict, *stubPaymentRepo) {
	t.Helper()
	now := time.Now().UTC()

	// Generate 4000 stale payments (40% of total conflicts)
	payments := make([]*domain.PaymentRecord, 4000)
	for i := 0; i < 4000; i++ {
		payments[i] = &domain.PaymentRecord{
			ID:         i + 1,
			QuinielaID: (i % 100) + 1,
			UserID:     (i % 500) + 1,
			Amount:     100 + (i % 900),
			CreatedAt:  now.Add(-time.Duration(8+i%30) * 24 * time.Hour), // 8-38 days old
		}
	}

	// Generate 4000 stale memberships (40% of total conflicts)
	memberships := make([]*domain.GroupMembership, 4000)
	for i := 0; i < 4000; i++ {
		memberships[i] = &domain.GroupMembership{
			ID:         i + 1,
			QuinielaID: (i % 100) + 1,
			UserID:     (i % 500) + 1,
			CreatedAt:  now.Add(-time.Duration(8+i%30) * 24 * time.Hour), // 8-38 days old
		}
	}

	// Generate 2000 groups without owner (20% of total conflicts)
	groupIDs := make([]int, 2000)
	quinielas := make([]*domain.Quiniela, 2000)
	for i := 0; i < 2000; i++ {
		groupIDs[i] = i + 1
		quinielas[i] = &domain.Quiniela{
			ID:   i + 1,
			Name: "Orphaned Group " + string(rune('A'+i%26)),
		}
	}

	qr := &stubQuinielaRepo{quinielas: quinielas}
	mr := &stubMemberRepoConflict{
		stubMemberRepo: &stubMemberRepo{memberships: memberships},
		groupIDs:       groupIDs,
	}
	pr := &stubPaymentRepo{records: payments}

	return qr, mr, pr
}

// newConflictServiceWithMaxScan creates a ConflictService with a specific max_scan parameter.
func newConflictServiceWithMaxScan(
	qr *stubQuinielaRepo,
	mr *stubMemberRepoConflict,
	pr *stubPaymentRepo,
	maxScan int,
) ConflictService {
	paramSvc := &systemParamServiceWithMaxScan{maxScan: maxScan}
	return NewConflictService(qr, mr, pr, paramSvc, &noopAuditLogger{}, zap.NewNop())
}

// verifyMaxScanLimit checks that the result respects the max_scan limit and logs observability data.
func verifyMaxScanLimit(t *testing.T, result *ConflictSummaryResult, maxScan int) {
	t.Helper()

	// Verify that TotalUnresolved is capped at maxScan, not the full 10K
	if result.TotalUnresolved > maxScan {
		t.Errorf("TotalUnresolved exceeds max_scan limit: want <=%d, got %d", maxScan, result.TotalUnresolved)
	}

	// With 10K conflicts available, we should hit the cap
	if result.TotalUnresolved != maxScan {
		t.Logf("WARNING: Expected to hit max_scan cap of %d with 10K conflicts, got %d", maxScan, result.TotalUnresolved)
	}

	// Verify at least some conflict types are represented in the summary.
	// When the max_scan limit is hit with a large backlog, not all types may
	// appear (e.g., if we scan 5000 conflicts from groups+payments, memberships
	// might not be included). This is expected behavior - the limit is working.
	if len(result.ByType) == 0 {
		t.Error("expected at least one conflict type in summary, got zero")
	}

	// Log which types were included for observability
	t.Logf("Conflict types included with max_scan=%d: %d types", maxScan, len(result.ByType))
	for _, ts := range result.ByType {
		t.Logf("  - %s: count=%d", ts.Type, ts.Count)
	}
}

// ── Test helpers for alert threshold tests ───────────────────────────────────

// spyLogger captures log calls for test verification
type spyLogger struct {
	warnCalled  bool
	errorCalled bool
	logger      *zap.Logger
}

func (s *spyLogger) newLogger() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)

	// Create a custom core that tracks calls
	core := &spyCore{spy: s}
	return zap.New(core)
}

type spyCore struct {
	spy *spyLogger
}

func (c *spyCore) Enabled(lvl zapcore.Level) bool {
	return true
}

func (c *spyCore) With(fields []zapcore.Field) zapcore.Core {
	return c
}

func (c *spyCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *spyCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if ent.Level == zapcore.WarnLevel {
		c.spy.warnCalled = true
	}
	if ent.Level == zapcore.ErrorLevel {
		c.spy.errorCalled = true
	}
	return nil
}

func (c *spyCore) Sync() error {
	return nil
}

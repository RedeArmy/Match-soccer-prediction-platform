package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
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

	// ownerless groups
	groupIDs := []int{1, 2}
	quinielas := []*domain.Quiniela{
		{ID: 1, Name: "Liga 1"},
		{ID: 2, Name: "Liga 2"},
	}

	// stale payment
	payment := &domain.PaymentRecord{
		ID:         10,
		QuinielaID: 1,
		UserID:     99,
		Amount:     500,
		CreatedAt:  now.Add(-72 * time.Hour),
	}

	// stale membership
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

	svc := NewConflictService(qr, mr, pr, &noopAuditLogger{}, zap.NewNop())

	conflicts, err := svc.ListConflicts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 4 {
		t.Fatalf("expected 4 conflicts (2 groups + 1 payment + 1 membership), got %d", len(conflicts))
	}

	var (
		foundNoOwner     bool
		foundStalePay    bool
		foundStaleMember bool
	)
	for _, c := range conflicts {
		switch c.Type {
		case domain.ConflictGroupNoOwner:
			foundNoOwner = true
			if c.EntityType != "quiniela" {
				t.Errorf("ConflictGroupNoOwner EntityType: want quiniela, got %q", c.EntityType)
			}
			if c.Details == nil {
				t.Error("ConflictGroupNoOwner Details: want non-nil")
			}
		case domain.ConflictPaymentStale:
			foundStalePay = true
			if c.EntityID != payment.ID {
				t.Errorf("ConflictPaymentStale EntityID: want %d, got %d", payment.ID, c.EntityID)
			}
			if c.EntityType != "payment_record" {
				t.Errorf("ConflictPaymentStale EntityType: want payment_record, got %q", c.EntityType)
			}
			if got, ok := c.Details["age_days"].(int); ok && got <= 0 {
				t.Errorf("ConflictPaymentStale age_days: want >0, got %d", got)
			}
		case domain.ConflictMembershipStale:
			foundStaleMember = true
			if c.EntityID != membership.ID {
				t.Errorf("ConflictMembershipStale EntityID: want %d, got %d", membership.ID, c.EntityID)
			}
			if c.EntityType != "group_membership" {
				t.Errorf("ConflictMembershipStale EntityType: want group_membership, got %q", c.EntityType)
			}
		}
	}
	if !foundNoOwner || !foundStalePay || !foundStaleMember {
		t.Errorf("expected all conflict categories; got noOwner=%v stalePay=%v staleMember=%v", foundNoOwner, foundStalePay, foundStaleMember)
	}
}

func TestConflictService_ListConflicts_NoConflicts_ReturnsEmptySlice(t *testing.T) {
	qr := &stubQuinielaRepo{quinielas: nil}
	mr := &stubMemberRepoConflict{stubMemberRepo: &stubMemberRepo{memberships: nil}, groupIDs: nil}
	pr := &stubPaymentRepo{records: nil}
	svc := NewConflictService(qr, mr, pr, &noopAuditLogger{}, zap.NewNop())

	conflicts, err := svc.ListConflicts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflicts == nil || len(conflicts) != 0 {
		t.Errorf("expected empty slice, got %v", conflicts)
	}
}

func TestConflictService_ResolveConflict_LogsAuditEntry(t *testing.T) {
	audit := &spyAuditLogger{}
	svc := NewConflictService(&stubQuinielaRepo{}, &stubMemberRepo{}, &stubPaymentRepo{}, audit, zap.NewNop())

	if err := svc.ResolveConflict(context.Background(), string(domain.ConflictGroupNoOwner), 7, 99, "ack"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !audit.called {
		t.Fatal("expected audit logger to be called")
	}
	if audit.action != "conflict.resolved" {
		t.Errorf("audit action: want conflict.resolved, got %q", audit.action)
	}
	if audit.metadata == nil || audit.metadata["conflict_type"] != string(domain.ConflictGroupNoOwner) {
		t.Errorf("audit metadata conflict_type: want %q, got %v", string(domain.ConflictGroupNoOwner), audit.metadata)
	}
}

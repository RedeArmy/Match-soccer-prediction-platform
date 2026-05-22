package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func TestPushDigestGate_P0P1_AlwaysBypass(t *testing.T) {
	g := notification.NewPushDigestGate(300, 1) // threshold=1: any P2+ triggers digest on 2nd
	ctx := context.Background()
	now := time.Now()

	for _, p := range []notification.Priority{notification.PriorityP0Critical, notification.PriorityP1High} {
		for i := range 10 {
			send, count := g.Record(ctx, 42, p, now)
			if !send {
				t.Errorf("priority %d iteration %d: expected bypass (sendIndividual=true), got false", p, i)
			}
			if count != 0 {
				t.Errorf("priority %d iteration %d: expected digestCount=0, got %d", p, i, count)
			}
		}
	}
}

func TestPushDigestGate_P2_UnderThreshold_SendsIndividual(t *testing.T) {
	g := notification.NewPushDigestGate(300, 5)
	ctx := context.Background()
	now := time.Now()

	for i := range 5 {
		send, count := g.Record(ctx, 1, notification.PriorityP2Medium, now)
		if !send {
			t.Errorf("iteration %d: expected sendIndividual=true, got false", i)
		}
		if count != 0 {
			t.Errorf("iteration %d: expected digestCount=0, got %d", i, count)
		}
	}
}

func TestPushDigestGate_P2_FirstOverflow_SendsDigest(t *testing.T) {
	g := notification.NewPushDigestGate(300, 5)
	ctx := context.Background()
	now := time.Now()

	for range 5 {
		g.Record(ctx, 1, notification.PriorityP2Medium, now) //nolint:errcheck
	}

	// 6th call: first overflow
	send, count := g.Record(ctx, 1, notification.PriorityP2Medium, now)
	if send {
		t.Error("6th push: expected sendIndividual=false (digest), got true")
	}
	if count != 6 {
		t.Errorf("6th push: expected digestCount=6, got %d", count)
	}
}

func TestPushDigestGate_P2_SubsequentOverflow_DropsEvent(t *testing.T) {
	g := notification.NewPushDigestGate(300, 5)
	ctx := context.Background()
	now := time.Now()

	for range 6 {
		g.Record(ctx, 1, notification.PriorityP2Medium, now) //nolint:errcheck
	}

	// 7th+ calls: drop silently
	for i := 7; i <= 10; i++ {
		send, count := g.Record(ctx, 1, notification.PriorityP2Medium, now)
		if send {
			t.Errorf("call %d: expected drop (sendIndividual=false, digestCount=0)", i)
		}
		if count != 0 {
			t.Errorf("call %d: expected digestCount=0, got %d", i, count)
		}
	}
}

func TestPushDigestGate_WindowExpiry_ResetsCount(t *testing.T) {
	g := notification.NewPushDigestGate(1, 2) // 1-second window
	ctx := context.Background()
	now := time.Now()

	// exhaust the window
	for range 3 {
		g.Record(ctx, 1, notification.PriorityP2Medium, now)
	}

	// advance past the window
	future := now.Add(2 * time.Second)
	send, count := g.Record(ctx, 1, notification.PriorityP2Medium, future)
	if !send {
		t.Error("after window expiry: expected sendIndividual=true (fresh window)")
	}
	if count != 0 {
		t.Errorf("after window expiry: expected digestCount=0, got %d", count)
	}
}

func TestPushDigestGate_IndependentUsersDoNotInterfere(t *testing.T) {
	g := notification.NewPushDigestGate(300, 2)
	ctx := context.Background()
	now := time.Now()

	// Exhaust user 1's window
	for range 3 {
		g.Record(ctx, 1, notification.PriorityP2Medium, now)
	}

	// User 2 should still be in the individual-delivery phase
	send, count := g.Record(ctx, 2, notification.PriorityP2Medium, now)
	if !send {
		t.Error("user 2: expected sendIndividual=true (separate window from user 1)")
	}
	if count != 0 {
		t.Errorf("user 2: expected digestCount=0, got %d", count)
	}
}

func TestPushDigestGate_Prune_RemovesExpiredWindows(t *testing.T) {
	g := notification.NewPushDigestGate(1, 5)
	ctx := context.Background()
	now := time.Now()

	g.Record(ctx, 1, notification.PriorityP2Medium, now)
	g.Record(ctx, 2, notification.PriorityP2Medium, now)

	// Prune after window expires — windows should be cleared
	g.Prune(now.Add(2 * time.Second))

	// Both users should get fresh windows (count resets to 1)
	send1, _ := g.Record(ctx, 1, notification.PriorityP2Medium, now.Add(2*time.Second))
	send2, _ := g.Record(ctx, 2, notification.PriorityP2Medium, now.Add(2*time.Second))
	if !send1 || !send2 {
		t.Error("after Prune: expected fresh windows for both users")
	}
}

func TestPushDigestGate_P3Low_GatedLikeP2(t *testing.T) {
	g := notification.NewPushDigestGate(300, 1)
	ctx := context.Background()
	now := time.Now()

	// First P3 push: individual
	send, _ := g.Record(ctx, 99, notification.PriorityP3Low, now)
	if !send {
		t.Error("first P3 push: expected sendIndividual=true")
	}

	// Second P3 push: digest
	send, count := g.Record(ctx, 99, notification.PriorityP3Low, now)
	if send {
		t.Error("second P3 push: expected digest (sendIndividual=false)")
	}
	if count != 2 {
		t.Errorf("second P3 push: expected digestCount=2, got %d", count)
	}
}

func TestPushDigestGate_SatisfiesDigestGateInterface(t *testing.T) {
	var _ notification.DigestGate = (*notification.PushDigestGate)(nil)
}

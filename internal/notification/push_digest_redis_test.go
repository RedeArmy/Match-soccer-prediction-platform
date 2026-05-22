package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

func newRedisGate(t *testing.T, windowSec int64, threshold int32) (*miniredis.Miniredis, *notification.RedisPushDigestGate) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return mr, notification.NewRedisPushDigestGate(rc, windowSec, threshold)
}

func TestRedisPushDigestGate_P0P1_AlwaysBypass(t *testing.T) {
	_, gate := newRedisGate(t, 300, 1)
	ctx := context.Background()
	now := time.Now()

	for _, p := range []notification.Priority{notification.PriorityP0Critical, notification.PriorityP1High} {
		for i := range 10 {
			send, count := gate.Record(ctx, 42, p, now)
			if !send || count != 0 {
				t.Errorf("priority %d iter %d: got (%v,%d); want (true,0)", p, i, send, count)
			}
		}
	}
}

func TestRedisPushDigestGate_P0P1_DoesNotIncrementCounter(t *testing.T) {
	_, gate := newRedisGate(t, 300, 2)
	ctx := context.Background()
	now := time.Now()

	for range 10 {
		gate.Record(ctx, 1, notification.PriorityP0Critical, now) //nolint:errcheck
	}

	// P2 should still be in the individual path (counter is zero, not 10).
	send, _ := gate.Record(ctx, 1, notification.PriorityP2Medium, now)
	if !send {
		t.Error("P2 after P0 spam: expected individual delivery (P0/P1 must not increment counter)")
	}
}

func TestRedisPushDigestGate_UnderThreshold_SendsIndividual(t *testing.T) {
	_, gate := newRedisGate(t, 300, 5)
	ctx := context.Background()
	now := time.Now()

	for i := range 5 {
		send, count := gate.Record(ctx, 1, notification.PriorityP2Medium, now)
		if !send || count != 0 {
			t.Errorf("iter %d: got (%v,%d); want (true,0)", i, send, count)
		}
	}
}

func TestRedisPushDigestGate_FirstOverflow_ReturnsDigestCount(t *testing.T) {
	_, gate := newRedisGate(t, 300, 5)
	ctx := context.Background()
	now := time.Now()

	for range 5 {
		gate.Record(ctx, 1, notification.PriorityP2Medium, now) //nolint:errcheck
	}

	send, count := gate.Record(ctx, 1, notification.PriorityP2Medium, now)
	if send {
		t.Error("6th push: expected sendIndividual=false (digest)")
	}
	if count != 6 {
		t.Errorf("6th push: expected digestCount=6, got %d", count)
	}
}

func TestRedisPushDigestGate_SubsequentOverflow_Drops(t *testing.T) {
	_, gate := newRedisGate(t, 300, 5)
	ctx := context.Background()
	now := time.Now()

	for range 6 {
		gate.Record(ctx, 1, notification.PriorityP2Medium, now) //nolint:errcheck
	}

	for i := 7; i <= 10; i++ {
		send, count := gate.Record(ctx, 1, notification.PriorityP2Medium, now)
		if send || count != 0 {
			t.Errorf("call %d: expected drop (false,0); got (%v,%d)", i, send, count)
		}
	}
}

func TestRedisPushDigestGate_WindowExpiry_ResetsCounter(t *testing.T) {
	mr, gate := newRedisGate(t, 5, 2)
	ctx := context.Background()
	now := time.Now()

	for range 3 {
		gate.Record(ctx, 1, notification.PriorityP2Medium, now) //nolint:errcheck
	}

	mr.FastForward(6 * time.Second) // advance past windowSec=5

	send, count := gate.Record(ctx, 1, notification.PriorityP2Medium, now)
	if !send || count != 0 {
		t.Errorf("after expiry: expected (true,0); got (%v,%d)", send, count)
	}
}

func TestRedisPushDigestGate_IndependentUsers_DoNotInterfere(t *testing.T) {
	_, gate := newRedisGate(t, 300, 2)
	ctx := context.Background()
	now := time.Now()

	for range 3 {
		gate.Record(ctx, 1, notification.PriorityP2Medium, now) //nolint:errcheck
	}

	send, count := gate.Record(ctx, 2, notification.PriorityP2Medium, now)
	if !send || count != 0 {
		t.Errorf("user 2: expected (true,0); got (%v,%d)", send, count)
	}
}

func TestRedisPushDigestGate_RedisError_DegradesToIndividual(t *testing.T) {
	mr, gate := newRedisGate(t, 300, 5)
	mr.Close() // simulate Redis outage
	ctx := context.Background()

	send, count := gate.Record(ctx, 99, notification.PriorityP2Medium, time.Now())
	if !send || count != 0 {
		t.Errorf("on Redis error: expected graceful degradation (true,0); got (%v,%d)", send, count)
	}
}

func TestRedisPushDigestGate_P3Low_GatedLikeP2(t *testing.T) {
	_, gate := newRedisGate(t, 300, 1)
	ctx := context.Background()
	now := time.Now()

	send, _ := gate.Record(ctx, 99, notification.PriorityP3Low, now)
	if !send {
		t.Error("first P3 push: expected sendIndividual=true")
	}

	send, count := gate.Record(ctx, 99, notification.PriorityP3Low, now)
	if send {
		t.Error("second P3 push: expected digest (sendIndividual=false)")
	}
	if count != 2 {
		t.Errorf("second P3 push: expected digestCount=2, got %d", count)
	}
}

func TestRedisPushDigestGate_ThresholdZero_FirstPushTriggersDigest(t *testing.T) {
	_, gate := newRedisGate(t, 300, 0)
	ctx := context.Background()
	now := time.Now()

	// With threshold=0, even the first P2/P3 push overflows (count=1 > 0).
	send, count := gate.Record(ctx, 1, notification.PriorityP2Medium, now)
	if send {
		t.Error("threshold=0: expected sendIndividual=false on first push")
	}
	if count != 1 {
		t.Errorf("threshold=0: expected digestCount=1, got %d", count)
	}
}

func TestRedisPushDigestGate_SatisfiesDigestGateInterface(t *testing.T) {
	var _ notification.DigestGate = (*notification.RedisPushDigestGate)(nil)
}

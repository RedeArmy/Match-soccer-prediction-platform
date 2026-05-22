package notification

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkPushDigestGate_BelowThreshold measures the common case: a user's
// first push within the window (count ≤ threshold → sendIndividual=true).
func BenchmarkPushDigestGate_BelowThreshold(b *testing.B) {
	gate := NewPushDigestGate(300, 5)
	now := time.Now()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		uid := 1
		for pb.Next() {
			// Each iteration uses the same user but the gate resets when the
			// window expires. With b.N >> 5 the gate will cycle through
			// individual → digest → drop → (window expires) → individual again.
			gate.Record(uid, PriorityP2Medium, now)
		}
	})
}

// BenchmarkPushDigestGate_P0Bypass measures the fast path: P0/P1 events bypass
// all gate state and return immediately. This is the most latency-sensitive path
// since P0 is used for security-critical notifications.
func BenchmarkPushDigestGate_P0Bypass(b *testing.B) {
	gate := NewPushDigestGate(300, 5)
	now := time.Now()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gate.Record(1, PriorityP0Critical, now)
		}
	})
}

// BenchmarkPushDigestGate_ManyUsers measures Record throughput when the gate
// tracks a large number of distinct users simultaneously. This exercises
// the map lookup and the lock contention pattern at scale.
func BenchmarkPushDigestGate_ManyUsers(b *testing.B) {
	const userCount = 10_000
	gate := NewPushDigestGate(300, 5)
	now := time.Now()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		uid := 0
		for pb.Next() {
			uid = (uid + 1) % userCount
			gate.Record(uid, PriorityP2Medium, now)
		}
	})
}

// BenchmarkPushDigestGate_Prune measures the cost of pruning expired windows.
// In production, Prune is called once per digest-window (every windowSec seconds)
// to prevent unbounded map growth. This benchmark fills the gate with 10 000
// users and then prunes all of them in a single call.
func BenchmarkPushDigestGate_Prune(b *testing.B) {
	const userCount = 10_000
	gate := NewPushDigestGate(300, 5)
	past := time.Now().Add(-10 * time.Minute) // all windows already expired

	for i := 0; i < userCount; i++ {
		gate.Record(i, PriorityP2Medium, past)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.Prune(time.Now())
		// Re-fill after each prune so the map is always full at the start of
		// the next iteration. Reset now so the new windows haven't expired yet.
		fresh := time.Now()
		for j := 0; j < userCount; j++ {
			gate.Record(j, PriorityP2Medium, fresh)
		}
	}
}

// BenchmarkPushDigestGate_ConcurrentMixedPriority measures lock contention
// when goroutines concurrently submit a realistic mix of priority levels
// across many users.
func BenchmarkPushDigestGate_ConcurrentMixedPriority(b *testing.B) {
	gate := NewPushDigestGate(300, 3)
	now := time.Now()
	priorities := []Priority{PriorityP0Critical, PriorityP1High, PriorityP2Medium, PriorityP3Low}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			uid := i % 1000
			p := priorities[i%len(priorities)]
			gate.Record(uid, p, now)
			i++
		}
	})
}

// BenchmarkPushDigestGate_SingleUser_OverThreshold measures the overflow path:
// a single user has exceeded the digest threshold and all subsequent P2/P3
// calls must return (false, 0) — the cheapest possible outcome after the count
// check.
func BenchmarkPushDigestGate_SingleUser_OverThreshold(b *testing.B) {
	const threshold = int32(3)
	gate := NewPushDigestGate(300, threshold)
	now := time.Now()

	// Push user past threshold before timing starts.
	for i := int32(0); i <= threshold+5; i++ {
		gate.Record(1, PriorityP2Medium, now)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gate.Record(1, PriorityP2Medium, now)
		}
	})
}

// BenchmarkPriorityOf exercises the EventType→Priority lookup table used
// before every notification dispatch to determine channel routing. A cache-miss
// on the map is the worst case; we use all known event types in rotation.
func BenchmarkPriorityOf(b *testing.B) {
	eventTypes := []EventType{
		EventPaymentBankTransferApproved,
		EventWithdrawalApproved,
		EventPredictionDeadlineApproach,
		EventGroupMemberJoined,
		EventAdminDailySummary,
		EventType(fmt.Sprintf("unknown.event%d", 99)), // exercises default path
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = PriorityOf(eventTypes[i%len(eventTypes)])
			i++
		}
	})
}

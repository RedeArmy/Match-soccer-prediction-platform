package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

func newTestRedisClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	return s, c
}

func TestRedisDLQService_Stats_ReturnsCountsAndSample(t *testing.T) {
	_, client := newTestRedisClient(t)
	svc := NewRedisDLQService(client, []events.EventType{events.EventPredictionMade}, 5, 10, zap.NewNop())

	now := time.Now().UTC().Truncate(time.Second)
	entry := dlqEntry{
		EventType: string(events.EventPredictionMade),
		Envelope: events.Envelope{
			Type:       events.EventPredictionMade,
			OccurredAt: now.Add(-time.Minute),
			Payload:    map[string]any{"id": 1},
		},
		Error:          "handler failed",
		DeadLetteredAt: now.Add(-2 * time.Hour),
		Attempts:       3,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := client.RPush(context.Background(), dlqKey(events.EventPredictionMade), string(raw)).Err(); err != nil {
		t.Fatalf("rpush: %v", err)
	}

	stats, err := svc.Stats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].Count != 1 {
		t.Errorf("Count: want 1, got %d", stats[0].Count)
	}
	if stats[0].OldestAt == nil {
		t.Error("OldestAt: want non-nil")
	}
	if len(stats[0].Sample) != 1 {
		t.Fatalf("Sample: want 1 entry, got %d", len(stats[0].Sample))
	}
	if stats[0].Sample[0].HandlerErr != entry.Error {
		t.Errorf("Sample HandlerErr: want %q, got %q", entry.Error, stats[0].Sample[0].HandlerErr)
	}
}

func TestRedisDLQService_Replay_MovesEntriesToStream(t *testing.T) {
	_, client := newTestRedisClient(t)
	svc := NewRedisDLQService(client, []events.EventType{events.EventMatchFinished}, 5, 10, zap.NewNop())

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 2; i++ {
		entry := dlqEntry{
			EventType: string(events.EventMatchFinished),
			Envelope: events.Envelope{
				Type:       events.EventMatchFinished,
				OccurredAt: now.Add(-time.Duration(i) * time.Minute),
				Payload:    map[string]any{"match_id": i + 1},
			},
			Error:          "fail",
			DeadLetteredAt: now.Add(-time.Hour),
			Attempts:       1,
		}
		raw, _ := json.Marshal(entry)
		_ = client.RPush(context.Background(), dlqKey(events.EventMatchFinished), string(raw)).Err()
	}

	replayed, err := svc.Replay(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replayed != 1 {
		t.Errorf("expected replayed=1, got %d", replayed)
	}

	llen, _ := client.LLen(context.Background(), dlqKey(events.EventMatchFinished)).Result()
	if llen != 1 {
		t.Errorf("expected DLQ length=1 after replay, got %d", llen)
	}

	xlen, err := client.XLen(context.Background(), streamKey(events.EventMatchFinished)).Result()
	if err != nil {
		t.Fatalf("xlen: %v", err)
	}
	if xlen != 1 {
		t.Errorf("expected stream length=1 after replay, got %d", xlen)
	}
}

func TestNewRedisDLQService_DefaultsWhenZeroOrNegative(t *testing.T) {
	_, client := newTestRedisClient(t)
	svc := NewRedisDLQService(client, nil, 0, 0, zap.NewNop())
	if svc.sampleSize != defaultDLQSampleSize {
		t.Errorf("sampleSize: want %d, got %d", defaultDLQSampleSize, svc.sampleSize)
	}
	if svc.replayDefaultLimit != defaultDLQReplayDefaultLimit {
		t.Errorf("replayDefaultLimit: want %d, got %d", defaultDLQReplayDefaultLimit, svc.replayDefaultLimit)
	}

	svc2 := NewRedisDLQService(client, nil, -1, -1, zap.NewNop())
	if svc2.sampleSize != defaultDLQSampleSize {
		t.Errorf("sampleSize (negative): want %d, got %d", defaultDLQSampleSize, svc2.sampleSize)
	}
	if svc2.replayDefaultLimit != defaultDLQReplayDefaultLimit {
		t.Errorf("replayDefaultLimit (negative): want %d, got %d", defaultDLQReplayDefaultLimit, svc2.replayDefaultLimit)
	}
}

func TestRedisDLQService_Purge_DeletesKeysAndReturnsTotal(t *testing.T) {
	_, client := newTestRedisClient(t)
	types := []events.EventType{events.EventMatchStarted, events.EventPredictionMade}
	svc := NewRedisDLQService(client, types, 5, 10, zap.NewNop())

	_ = client.RPush(context.Background(), dlqKey(events.EventMatchStarted), "a", "b").Err()
	_ = client.RPush(context.Background(), dlqKey(events.EventPredictionMade), "c").Err()

	purged, err := svc.Purge(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if purged != 3 {
		t.Errorf("expected purged=3, got %d", purged)
	}
	for _, et := range types {
		llen, _ := client.LLen(context.Background(), dlqKey(et)).Result()
		if llen != 0 {
			t.Errorf("expected %s DLQ length=0 after purge, got %d", et, llen)
		}
	}
}

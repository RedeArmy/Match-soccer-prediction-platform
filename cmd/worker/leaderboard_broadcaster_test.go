package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// stubMemberRepo implements ActiveMemberLister for broadcaster tests.
type stubMemberRepo struct {
	// membersByQuiniela maps quinielaID → active member IDs
	membersByQuiniela map[int][]int
	err               error
}

func (r *stubMemberRepo) ListActiveMemberIDsByGroup(_ context.Context, quinielaID int) ([]int, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.membersByQuiniela[quinielaID], nil
}

var _ ActiveMemberLister = (*stubMemberRepo)(nil)

// newMiniredisClient starts an in-process miniredis and returns a connected client.
func newMiniredisClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return mr, rc
}

// subscribeChannel subscribes to a Redis channel and returns a channel of received messages.
func subscribeChannel(t *testing.T, rc *redis.Client, channel string) <-chan string {
	t.Helper()
	pubsub := rc.Subscribe(context.Background(), channel)
	t.Cleanup(func() { _ = pubsub.Close() })
	out := make(chan string, 32)
	go func() {
		for msg := range pubsub.Channel() {
			out <- msg.Payload
		}
	}()
	return out
}

func TestLeaderboardBroadcaster_Noop_DoesNotPanic(t *testing.T) {
	var b LeaderboardBroadcaster = noopLeaderboardBroadcaster{}
	b.BroadcastLeaderboardUpdated(context.Background(), []int{1, 2, 3})
}

func TestLeaderboardBroadcaster_EmptyQuinielaIDs_PublishesNothing(t *testing.T) {
	mr, rc := newMiniredisClient(t)
	msgs := subscribeChannel(t, rc, "user_notifications")

	b := &redisPubLeaderboardBroadcaster{
		client:     rc,
		memberRepo: &stubMemberRepo{},
		log:        zap.NewNop(),
	}
	b.BroadcastLeaderboardUpdated(context.Background(), nil)

	if len(mr.DB(0).Keys()) > 0 {
		t.Error("expected no Redis activity for empty quinielaIDs")
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestLeaderboardBroadcaster_PublishesOneMessagePerMember(t *testing.T) {
	_, rc := newMiniredisClient(t)

	// subscribe before broadcasting so no message is missed
	pubsub := rc.Subscribe(context.Background(), "user_notifications")
	t.Cleanup(func() { _ = pubsub.Close() })
	received := pubsub.Channel()

	members := map[int][]int{
		10: {101, 102, 103},
		20: {201},
	}
	b := &redisPubLeaderboardBroadcaster{
		client:     rc,
		memberRepo: &stubMemberRepo{membersByQuiniela: members},
		log:        zap.NewNop(),
	}
	b.BroadcastLeaderboardUpdated(context.Background(), []int{10, 20})

	// Expect 4 messages total: 3 for quiniela 10 + 1 for quiniela 20.
	// Use a timeout rather than a default case: the Redis pubsub channel is
	// delivered asynchronously so messages may not be buffered yet even though
	// BroadcastLeaderboardUpdated has returned.
	got := make([]leaderboardSignalPayload, 0, 4)
	for i := 0; i < 4; i++ {
		select {
		case msg := <-received:
			var sig leaderboardSignalPayload
			if err := json.Unmarshal([]byte(msg.Payload), &sig); err != nil {
				t.Fatalf("unmarshal signal: %v", err)
			}
			got = append(got, sig)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for message %d/4", i+1)
		}
	}

	for _, sig := range got {
		if sig.EventType != string(notification.EventLeaderboardUpdated) {
			t.Errorf("event_type = %q; want %q", sig.EventType, notification.EventLeaderboardUpdated)
		}
		if sig.ID != 0 {
			t.Errorf("id should be 0 (synthetic signal), got %d", sig.ID)
		}
		if sig.Title != "" || sig.Body != "" {
			t.Errorf("title/body should be empty for leaderboard signal, got %q / %q", sig.Title, sig.Body)
		}
		if sig.CreatedAt == "" {
			t.Error("created_at must not be empty")
		}
	}
}

func TestLeaderboardBroadcaster_ActionURLContainsQuinielaID(t *testing.T) {
	_, rc := newMiniredisClient(t)
	pubsub := rc.Subscribe(context.Background(), "user_notifications")
	t.Cleanup(func() { _ = pubsub.Close() })
	received := pubsub.Channel()

	b := &redisPubLeaderboardBroadcaster{
		client: rc,
		memberRepo: &stubMemberRepo{membersByQuiniela: map[int][]int{
			42: {999},
		}},
		log: zap.NewNop(),
	}
	b.BroadcastLeaderboardUpdated(context.Background(), []int{42})

	select {
	case msg := <-received:
		var sig leaderboardSignalPayload
		if err := json.Unmarshal([]byte(msg.Payload), &sig); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		want := "/api/v1/groups/42/leaderboard"
		if sig.ActionURL != want {
			t.Errorf("action_url = %q; want %q", sig.ActionURL, want)
		}
		if sig.UserID != 999 {
			t.Errorf("user_id = %d; want 999", sig.UserID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for leaderboard signal")
	}
}

func TestLeaderboardBroadcaster_MemberRepoError_DoesNotPropagate(t *testing.T) {
	mr, rc := newMiniredisClient(t)

	b := &redisPubLeaderboardBroadcaster{
		client: rc,
		memberRepo: &stubMemberRepo{
			err: context.DeadlineExceeded,
		},
		log: zap.NewNop(),
	}
	// Must not panic or return an error.
	b.BroadcastLeaderboardUpdated(context.Background(), []int{1, 2, 3})

	// No messages should have been published.
	if n := len(mr.DB(0).Keys()); n != 0 {
		t.Errorf("expected 0 Redis keys after member-repo error, got %d", n)
	}
}

func TestLeaderboardBroadcaster_EmptyMemberList_PublishesNothing(t *testing.T) {
	mr, rc := newMiniredisClient(t)

	b := &redisPubLeaderboardBroadcaster{
		client: rc,
		// quiniela 7 has no active members
		memberRepo: &stubMemberRepo{membersByQuiniela: map[int][]int{7: {}}},
		log:        zap.NewNop(),
	}
	b.BroadcastLeaderboardUpdated(context.Background(), []int{7})

	if n := len(mr.DB(0).Keys()); n != 0 {
		t.Errorf("expected 0 Redis keys for empty member list, got %d", n)
	}
}

func TestLeaderboardBroadcaster_PublishError_DoesNotPropagate(t *testing.T) {
	_, rc := newMiniredisClient(t)
	// Close the client before broadcasting to force a Publish error.
	_ = rc.Close()

	b := &redisPubLeaderboardBroadcaster{
		client:     rc,
		memberRepo: &stubMemberRepo{membersByQuiniela: map[int][]int{1: {99}}},
		log:        zap.NewNop(),
	}
	// Must not panic or propagate the Publish error.
	b.BroadcastLeaderboardUpdated(context.Background(), []int{1})
}

func TestLeaderboardBroadcaster_ContextCancelled_AbortsRetry(t *testing.T) {
	_, rc := newMiniredisClient(t)
	_ = rc.Close() // force every Publish call to fail so the retry loop is entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel: ctx.Done() fires immediately inside the retry select

	b := &redisPubLeaderboardBroadcaster{
		client:     rc,
		memberRepo: &stubMemberRepo{membersByQuiniela: map[int][]int{1: {99}}},
		log:        zap.NewNop(),
	}
	// Must complete without blocking; the cancelled context exits the retry loop
	// via the ctx.Done() case rather than waiting for the full backoff sleep.
	b.BroadcastLeaderboardUpdated(ctx, []int{1})
}

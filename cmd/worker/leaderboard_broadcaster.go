package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// ActiveMemberLister is the narrow read capability that redisPubLeaderboardBroadcaster
// needs from the membership store. Accepting this interface rather than the full
// GroupMembershipRepository keeps the dependency surface minimal and makes
// unit tests straightforward to write.
type ActiveMemberLister interface {
	ListActiveMemberIDsByGroup(ctx context.Context, quinielaID int) ([]int, error)
}

// LeaderboardBroadcaster signals active group members via SSE that the
// leaderboard for their quiniela has been updated and should be refetched.
// Implementations are best-effort: errors are logged and not propagated so a
// failed signal never blocks the post-scoring pipeline.
type LeaderboardBroadcaster interface {
	BroadcastLeaderboardUpdated(ctx context.Context, quinielaIDs []int)
}

// noopLeaderboardBroadcaster satisfies LeaderboardBroadcaster and does nothing.
// Used in tests and as the fallback when Redis is unavailable.
type noopLeaderboardBroadcaster struct{}

func (noopLeaderboardBroadcaster) BroadcastLeaderboardUpdated(_ context.Context, _ []int) {}

// leaderboardSignalPayload is the JSON body published to the user_notifications
// Redis Pub/Sub channel. Its field names match hub.Notification and the bridge
// parse struct in server_bridge.go, so the API server delivers the event to
// connected SSE clients without any transformation.
//
// The ID field is 0 — this signal is not persisted to the notification outbox;
// it is a synthetic push whose sole purpose is to trigger a client-side refetch.
type leaderboardSignalPayload struct {
	UserID    int    `json:"user_id"`
	ID        int64  `json:"id"`
	EventType string `json:"event_type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionURL string `json:"action_url"`
	CreatedAt string `json:"created_at"`
}

// redisPubLeaderboardBroadcaster looks up the active members for each quiniela
// and publishes a leaderboard.updated signal to the user_notifications Redis
// Pub/Sub channel for every member. The API server's Redis bridge delivers the
// signal to any connected SSE client for that user, which triggers a fresh
// GET /api/v1/groups/{id}/leaderboard request.
//
// Cache invalidation always precedes this call (see postScoringWork), so the
// client's refetch arrives after the cache is cold and receives post-scoring data.
//
// Concurrency: quinielas are processed in parallel with a pool bounded by
// snapshotConcurrency (same limit as snapshot writes) to avoid saturating the
// database connection pool during elimination-phase multi-match scoring bursts.
type redisPubLeaderboardBroadcaster struct {
	client     redis.Cmdable
	memberRepo ActiveMemberLister
	log        *zap.Logger
}

func (b *redisPubLeaderboardBroadcaster) BroadcastLeaderboardUpdated(ctx context.Context, quinielaIDs []int) {
	if len(quinielaIDs) == 0 {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(snapshotConcurrency)

	for _, qid := range quinielaIDs {
		qid := qid
		g.Go(func() error {
			b.broadcastForQuiniela(gctx, qid, now)
			return nil
		})
	}
	_ = g.Wait()
}

// broadcastForQuiniela publishes a leaderboard.updated signal to every active
// member of quinielaID. Errors at each step are logged and swallowed.
func (b *redisPubLeaderboardBroadcaster) broadcastForQuiniela(ctx context.Context, quinielaID int, now string) {
	memberIDs, err := b.memberRepo.ListActiveMemberIDsByGroup(ctx, quinielaID)
	if err != nil {
		b.log.Warn("leaderboard broadcaster: fetch members failed",
			zap.Int("quiniela_id", quinielaID),
			zap.Error(err),
		)
		return
	}
	if len(memberIDs) == 0 {
		return
	}

	actionURL := fmt.Sprintf("/api/v1/groups/%d/leaderboard", quinielaID)
	for _, uid := range memberIDs {
		sig := leaderboardSignalPayload{
			UserID:    uid,
			ID:        0,
			EventType: string(notification.EventLeaderboardUpdated),
			ActionURL: actionURL,
			CreatedAt: now,
		}
		payload, _ := json.Marshal(sig) // plain struct; never errors
		if err := b.client.Publish(ctx, "user_notifications", string(payload)).Err(); err != nil {
			b.log.Warn("leaderboard broadcaster: redis publish failed",
				zap.Int("user_id", uid),
				zap.Int("quiniela_id", quinielaID),
				zap.Error(err),
			)
		}
	}
}

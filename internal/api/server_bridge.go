package api

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// runPgNotifyBridge holds a dedicated PostgreSQL connection and listens on the
// 'user_notifications' channel.  Each notification payload is parsed and
// broadcast to the in-process SSE hub so connected SSE clients receive it
// without a database round-trip.
//
// The goroutine exits when ctx is cancelled (server shutdown) or when the
// dedicated connection is lost.  Loss of the LISTEN connection means in-flight
// SSE clients will miss notifications until the next reconnect — acceptable
// because the client resynchronises on reconnect via GET /notifications.
func (s *Server) runPgNotifyBridge(ctx context.Context) {
	if s.db == nil || s.notifHub == nil {
		return
	}

	conn, err := s.db.Acquire(ctx)
	if err != nil {
		s.log.Warn("pg_notify bridge: failed to acquire connection", zap.Error(err))
		return
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN user_notifications"); err != nil {
		s.log.Warn("pg_notify bridge: LISTEN failed", zap.Error(err))
		return
	}
	s.log.Info("pg_notify bridge: listening on user_notifications")

	for {
		pgNotif, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // clean shutdown
			}
			s.log.Warn("pg_notify bridge: WaitForNotification error", zap.Error(err))
			return
		}

		var p struct {
			UserID    int    `json:"user_id"`
			ID        int64  `json:"id"`
			EventType string `json:"event_type"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			ActionURL string `json:"action_url"`
			CreatedAt string `json:"created_at"`
		}
		if err := json.Unmarshal([]byte(pgNotif.Payload), &p); err != nil {
			s.log.Warn("pg_notify bridge: failed to parse payload", zap.Error(err))
			continue
		}

		s.notifHub.Broadcast(p.UserID, hub.Notification{
			ID:        p.ID,
			UserID:    p.UserID,
			EventType: p.EventType,
			Title:     p.Title,
			Body:      p.Body,
			ActionURL: p.ActionURL,
			CreatedAt: p.CreatedAt,
		})
	}
}

// leaderboardTTLHook returns the mutation hook registered for
// ParamKeyCacheLeaderboardTTL. When the admin updates that param, the hook
// reads the fresh value and propagates it to ranker so the change takes effect
// immediately without a process restart.
func leaderboardTTLHook(paramSvc service.SystemParamService, ranker *service.CachedRankingService) func(context.Context) {
	return func(ctx context.Context) {
		newTTL := time.Duration(paramSvc.GetInt(
			ctx, domain.ParamKeyCacheLeaderboardTTL, domain.DefaultCacheLeaderboardTTLSeconds,
		)) * time.Second
		ranker.UpdateTTL(newTTL)
		ranker.InvalidateAll(ctx)
	}
}

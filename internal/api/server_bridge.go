package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/service"
)

const (
	bridgeBackoffInit = time.Second
	bridgeBackoffMax  = 30 * time.Second
)

// runPgNotifyBridge loops forever (until ctx is cancelled), maintaining a
// dedicated PostgreSQL connection that LISTENs on the 'user_notifications'
// channel. Each notification payload is parsed and broadcast to the in-process
// SSE hub so connected SSE clients receive it without a database round-trip.
//
// When the connection is lost the bridge reconnects with exponential backoff
// (1 s → 2 s → … → 30 s) so a transient PostgreSQL restart or network blip
// does not permanently silence the SSE channel.
func (s *Server) runPgNotifyBridge(ctx context.Context) {
	if s.db == nil || s.notifHub == nil {
		return
	}
	backoff := bridgeBackoffInit
	for {
		err := s.listenAndBridge(ctx)
		if ctx.Err() != nil {
			return // clean shutdown
		}
		s.log.Warn("pg_notify bridge: connection lost — reconnecting",
			zap.Error(err),
			zap.Duration("backoff", backoff),
		)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, bridgeBackoffMax)
	}
}

// listenAndBridge acquires one connection, sends LISTEN, and fans out
// notifications until the connection is lost or ctx is cancelled.
// Returns nil on clean shutdown (ctx cancelled); returns a non-nil error on
// connection loss, which runPgNotifyBridge uses to trigger a reconnect.
func (s *Server) listenAndBridge(ctx context.Context) error {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN user_notifications"); err != nil {
		return fmt.Errorf("LISTEN failed: %w", err)
	}
	s.log.Info("pg_notify bridge: listening on user_notifications")

	for {
		pgNotif, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("WaitForNotification: %w", err) // triggers reconnect
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

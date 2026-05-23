package api

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/internal/notification/hub"
)

// RunBridgeOnce calls listenAndBridge once. Used by integration tests to
// exercise the full LISTEN/NOTIFY path without going through StartPgNotifyBridge.
func (s *Server) RunBridgeOnce(ctx context.Context) error {
	return s.listenAndBridge(ctx)
}

// NotifHubForTest returns the SSE hub. The hub is created inside Routes();
// call Routes() before this accessor.
func (s *Server) NotifHubForTest() *hub.Hub {
	return s.notifHub
}

// RunRedisBridgeForTest calls runRedisBridge directly. Used by unit tests to
// exercise the Redis Pub/Sub bridge without going through StartRedisBridge.
func (s *Server) RunRedisBridgeForTest(ctx context.Context, rc redis.UniversalClient) {
	s.runRedisBridge(ctx, rc)
}

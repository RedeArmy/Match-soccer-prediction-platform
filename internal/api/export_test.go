package api

import (
	"context"

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

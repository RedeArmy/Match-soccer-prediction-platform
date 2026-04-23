package service

import (
	"context"
	"testing"
)

func TestNoopDLQService_ReturnsEmptyResults(t *testing.T) {
	var svc NoopDLQService

	stats, err := svc.Stats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats == nil || len(stats) != 0 {
		t.Errorf("expected empty stats slice, got %v", stats)
	}

	n, err := svc.Replay(context.Background(), 10)
	if err != nil || n != 0 {
		t.Errorf("expected replay=0 nil err, got %d %v", n, err)
	}

	purged, err := svc.Purge(context.Background())
	if err != nil || purged != 0 {
		t.Errorf("expected purge=0 nil err, got %d %v", purged, err)
	}
}

package service

import (
	"context"
	"time"
)

// DLQStat summarises the dead-letter queue for one event type.
type DLQStat struct {
	EventType string     `json:"event_type"`
	Count     int64      `json:"count"`
	OldestAt  *time.Time `json:"oldest_at,omitempty"`
	Sample    []DLQEntry `json:"sample"`
}

// DLQEntry is the payload of a single dead-lettered event, as returned by
// DLQService.Stats for inspection.
type DLQEntry struct {
	DeadLetteredAt time.Time      `json:"dead_lettered_at"`
	HandlerErr     string         `json:"handler_err"`
	Payload        map[string]any `json:"payload"`
}

// DLQService exposes management operations on the dead-letter queue.
// Implementations are driver-specific (Redis vs in-memory); pass a no-op
// implementation when the DLQ feature is not supported.
type DLQService interface {
	// Stats returns the count, oldest entry age, and a sample of messages
	// for each known event type.
	Stats(ctx context.Context) ([]DLQStat, error)
	// Replay re-enqueues up to limit entries from all DLQ keys back onto
	// their original streams. Returns the total number replayed.
	Replay(ctx context.Context, limit int) (int, error)
	// Purge deletes all entries from all DLQ keys.
	// Returns the total number of entries removed.
	Purge(ctx context.Context) (int64, error)
}

// NoopDLQService is a DLQService implementation used when the event bus driver
// does not support a persistent dead-letter queue (e.g. in-memory bus).
// All operations return empty results so admin handlers degrade gracefully.
type NoopDLQService struct{}

func (NoopDLQService) Stats(_ context.Context) ([]DLQStat, error)   { return []DLQStat{}, nil }
func (NoopDLQService) Replay(_ context.Context, _ int) (int, error) { return 0, nil }
func (NoopDLQService) Purge(_ context.Context) (int64, error)       { return 0, nil }

var _ DLQService = NoopDLQService{}

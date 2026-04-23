package service

import "context"

// NoopDLQService is a DLQService implementation used when the event bus driver
// does not support a persistent dead-letter queue (e.g. in-memory bus).
// All operations return empty results so admin handlers degrade gracefully.
type NoopDLQService struct{}

func (NoopDLQService) Stats(_ context.Context) ([]DLQStat, error)   { return []DLQStat{}, nil }
func (NoopDLQService) Replay(_ context.Context, _ int) (int, error) { return 0, nil }
func (NoopDLQService) Purge(_ context.Context) (int64, error)       { return 0, nil }

var _ DLQService = NoopDLQService{}

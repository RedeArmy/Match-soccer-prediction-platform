package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// dlqSampleSize is the maximum number of entries returned in the sample field
// of DLQStat. Kept small to bound response size when queues are large.
const dlqSampleSize = 5

// RedisDLQService implements service.DLQService against the Redis lists used by
// RedisBus. It is a separate type from RedisBus so that it can be injected into
// the HTTP server without granting full bus access to the admin layer.
type RedisDLQService struct {
	client     *redis.Client
	eventTypes []events.EventType
	log        *zap.Logger
}

// NewRedisDLQService constructs a RedisDLQService for the given event types.
// eventTypes must match the full set of EventType constants used by RedisBus so
// that all DLQ keys are covered by Stats and Purge.
func NewRedisDLQService(client *redis.Client, eventTypes []events.EventType, log *zap.Logger) *RedisDLQService {
	return &RedisDLQService{client: client, eventTypes: eventTypes, log: log}
}

// Stats returns the depth and a sample for each known DLQ key.
func (s *RedisDLQService) Stats(ctx context.Context) ([]service.DLQStat, error) {
	stats := make([]service.DLQStat, 0, len(s.eventTypes))
	for _, et := range s.eventTypes {
		key := dlqKey(et)
		count, err := s.client.LLen(ctx, key).Result()
		if err != nil {
			s.log.Warn("dlq: llen failed", zap.String("key", key), zap.Error(err))
			continue
		}

		stat := service.DLQStat{
			EventType: string(et),
			Count:     count,
		}

		if count > 0 {
			// Peek oldest entry (head of list) for age and sample
			raw, err := s.client.LRange(ctx, key, 0, int64(dlqSampleSize-1)).Result()
			if err == nil && len(raw) > 0 {
				var oldest dlqEntry
				if jsonErr := json.Unmarshal([]byte(raw[0]), &oldest); jsonErr == nil {
					t := oldest.DeadLetteredAt
					stat.OldestAt = &t
				}

				sample := make([]service.DLQEntry, 0, len(raw))
				for _, r := range raw {
					var e dlqEntry
					if jsonErr := json.Unmarshal([]byte(r), &e); jsonErr != nil {
						continue
					}
					sample = append(sample, service.DLQEntry{
						DeadLetteredAt: e.DeadLetteredAt,
						HandlerErr:     e.Error,
						Payload:        map[string]any{"event_type": e.EventType, "attempts": e.Attempts},
					})
				}
				stat.Sample = sample
			}
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// Replay pops up to limit entries from each DLQ key and re-publishes them
// onto their original Redis Streams. Returns the total number replayed.
func (s *RedisDLQService) Replay(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	replayed := 0
	for _, et := range s.eventTypes {
		key := dlqKey(et)
		for i := 0; i < limit; i++ {
			raw, err := s.client.LPop(ctx, key).Result()
			if err == redis.Nil {
				break
			}
			if err != nil {
				s.log.Warn("dlq replay: lpop failed", zap.String("key", key), zap.Error(err))
				break
			}

			var entry dlqEntry
			if err := json.Unmarshal([]byte(raw), &entry); err != nil {
				s.log.Warn("dlq replay: unmarshal failed", zap.Error(err))
				continue
			}

			payload, err := json.Marshal(entry.Envelope)
			if err != nil {
				s.log.Warn("dlq replay: marshal envelope failed", zap.Error(err))
				continue
			}

			if err := s.client.XAdd(ctx, &redis.XAddArgs{
				Stream: streamKey(et),
				MaxLen: streamMaxLen,
				Approx: true,
				Values: map[string]any{"payload": string(payload)},
			}).Err(); err != nil {
				s.log.Warn("dlq replay: xadd failed", zap.String("stream", streamKey(et)), zap.Error(err))
				continue
			}
			replayed++
		}
	}
	return replayed, nil
}

// Purge deletes all entries from all DLQ keys and returns the total removed.
func (s *RedisDLQService) Purge(ctx context.Context) (int64, error) {
	var total int64
	for _, et := range s.eventTypes {
		key := dlqKey(et)
		count, err := s.client.LLen(ctx, key).Result()
		if err != nil {
			s.log.Warn("dlq purge: llen failed", zap.String("key", key), zap.Error(err))
			continue
		}
		if count == 0 {
			continue
		}
		if err := s.client.Del(ctx, key).Err(); err != nil {
			s.log.Warn("dlq purge: del failed", zap.String("key", key), zap.Error(err))
			continue
		}
		total += count
	}
	return total, nil
}

// lastReplayed stores the timestamp of the most recent replay for observability.
// Not persisted; resets on process restart.
var _ = time.Now // ensure time is used

var _ service.DLQService = (*RedisDLQService)(nil)

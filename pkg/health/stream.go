package health

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// DLQChecker reports the number of unprocessed events sitting in the Redis
// dead-letter queue for a specific event type. A non-empty DLQ means that
// at least one event handler exhausted all retry attempts without succeeding
// - likely indicating a persistent failure (database down, bug in handler)
// that requires operator intervention.
//
// The check is informational rather than binary: it returns a Result with
// status "ok" when the DLQ is empty and status "error" with the entry count
// when it is not. Operators should wire an alert on the "error" state so they
// are notified promptly and can replay the dead-lettered events.
type DLQChecker struct {
	client    *redis.Client
	dlqKey    string
	eventType string
}

// NewDLQChecker creates a DLQChecker for the given event type. dlqKey must
// match the key used by messaging.RedisBus (format: "dlq:<event_type>").
func NewDLQChecker(client *redis.Client, eventType string) *DLQChecker {
	return &DLQChecker{
		client:    client,
		dlqKey:    "dlq:" + eventType,
		eventType: eventType,
	}
}

// Name returns a stable identifier used as the check key in the JSON response.
func (c *DLQChecker) Name() string {
	return "dlq:" + c.eventType
}

// Check queries the length of the DLQ list. A length of zero is healthy.
// Any entries indicate events that could not be processed after all retries.
func (c *DLQChecker) Check(ctx context.Context) Result {
	start := time.Now()
	n, err := c.client.LLen(ctx, c.dlqKey).Result()
	if err != nil {
		return Result{Status: "error", Error: fmt.Sprintf("LLEN %s: %v", c.dlqKey, err)}
	}
	latency := time.Since(start).Milliseconds()
	if n > 0 {
		return Result{
			Status:    "error",
			LatencyMS: latency,
			Error:     fmt.Sprintf("%d unprocessed event(s) in dead-letter queue - manual replay required", n),
		}
	}
	return Result{Status: "ok", LatencyMS: latency}
}

// StreamPendingChecker reports the number of messages in the Redis Streams
// pending-entry list (PEL) for a given consumer group and stream. A growing
// PEL signals that the worker is consuming events more slowly than they are
// published - or, in the worst case, that the worker is down entirely and
// messages are accumulating without being processed.
//
// Unlike DLQChecker, a non-zero PEL is not immediately alarming: messages
// spend a brief time in the PEL while being actively processed. The check
// becomes meaningful when PEL size is consistently greater than zero between
// health-check intervals, indicating worker lag.
type StreamPendingChecker struct {
	client    *redis.Client
	streamKey string
	group     string
	name      string
}

// NewStreamPendingChecker creates a checker for the pending-entry list of
// the given stream and consumer group. streamKey and group must match the
// values used by messaging.RedisBus ("stream:<event_type>", "quiniela-workers").
func NewStreamPendingChecker(client *redis.Client, streamKey, group string) *StreamPendingChecker {
	return &StreamPendingChecker{
		client:    client,
		streamKey: streamKey,
		group:     group,
		name:      "stream_pending:" + streamKey,
	}
}

// Name returns a stable identifier used as the check key in the JSON response.
func (c *StreamPendingChecker) Name() string { return c.name }

// Check uses XPENDING to count messages delivered to the consumer group but
// not yet acknowledged. Returns "ok" with the pending count in the error field
// for observability even when the count is zero.
func (c *StreamPendingChecker) Check(ctx context.Context) Result {
	start := time.Now()
	pending, err := c.client.XPending(ctx, c.streamKey, c.group).Result()
	if err != nil {
		// NOGROUP is returned when the stream or group does not yet exist
		// (worker has never started). That is not an error from an operator
		// standpoint - the group is created lazily on first Subscribe call.
		if isNoGroupError(err) {
			return Result{Status: "ok", LatencyMS: time.Since(start).Milliseconds()}
		}
		return Result{Status: "error", Error: fmt.Sprintf("XPENDING %s/%s: %v", c.streamKey, c.group, err)}
	}
	latency := time.Since(start).Milliseconds()
	if pending.Count > 0 {
		return Result{
			Status:    "error",
			LatencyMS: latency,
			Error:     fmt.Sprintf("%d message(s) pending in stream %s (group %s) - worker may be lagging or down", pending.Count, c.streamKey, c.group),
		}
	}
	return Result{Status: "ok", LatencyMS: latency}
}

// isNoGroupError reports whether err is the Redis NOGROUP error returned when
// XPENDING is called on a stream/group combination that does not exist.
func isNoGroupError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return len(s) >= 7 && s[:7] == "NOGROUP"
}

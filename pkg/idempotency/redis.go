package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisEntry is the on-disk (Redis) representation of an idempotency Entry.
// Short field names keep the stored value compact.
type redisEntry struct {
	State      EntryState          `json:"s"`
	StatusCode int                 `json:"c,omitempty"`
	Headers    map[string][]string `json:"h,omitempty"`
	Body       []byte              `json:"b,omitempty"`
}

func marshalEntry(e Entry) ([]byte, error) {
	re := redisEntry{
		State:      e.State,
		StatusCode: e.StatusCode,
		Body:       e.Body,
	}
	if len(e.Headers) > 0 {
		re.Headers = map[string][]string(e.Headers)
	}
	return json.Marshal(re)
}

func unmarshalEntry(data []byte) (Entry, bool) {
	var re redisEntry
	if err := json.Unmarshal(data, &re); err != nil {
		return Entry{}, false
	}
	return Entry{
		State:      re.State,
		StatusCode: re.StatusCode,
		Headers:    http.Header(re.Headers),
		Body:       re.Body,
	}, true
}

// RedisStore implements Store using Redis as the shared backing store.
//
// Reservations use SET NX, which is atomic at the Redis level — only one
// caller across all replicas can claim a key. This makes idempotency
// guarantees hold under horizontal scaling, unlike the MemoryStore which
// only scopes reservations within a single process.
//
// All operations propagate context cancellation so a timed-out or cancelled
// request does not block waiting for a slow Redis round-trip.
//
// The client field accepts redis.Cmdable so the store can be constructed with
// a *redis.Client, *redis.ClusterClient, or a miniredis stub in tests.
type RedisStore struct {
	client redis.Cmdable
}

// NewRedisStore constructs a RedisStore backed by the provided client.
// The client is not owned by the store; the caller is responsible for closing
// it after the store is no longer needed.
func NewRedisStore(client redis.Cmdable) *RedisStore {
	return &RedisStore{client: client}
}

// Load returns the Entry for key if it exists and has not expired.
// Corrupted entries (written by a different schema) are treated as a miss so
// the next caller can re-reserve the key with the current schema.
func (s *RedisStore) Load(ctx context.Context, key string) (Entry, bool, error) {
	data, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("idempotency: load %q: %w", key, err)
	}
	e, ok := unmarshalEntry(data)
	if !ok {
		// Treat corrupted data as a miss: the caller may re-reserve.
		return Entry{}, false, nil
	}
	return e, true, nil
}

// Reserve atomically creates an InFlight entry for key if none exists.
// Returns (true, nil) when the reservation is granted; (false, nil) when the
// key already exists (either InFlight from a concurrent request on any replica,
// or Committed from a prior completed request).
func (s *RedisStore) Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	data, err := marshalEntry(Entry{State: InFlight})
	if err != nil {
		return false, fmt.Errorf("idempotency: marshal in-flight entry: %w", err)
	}
	ok, err := s.client.SetNX(ctx, key, data, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency: reserve %q: %w", key, err)
	}
	return ok, nil
}

// Commit replaces the InFlight entry for key with the committed response.
// The TTL is reset to the provided value so the committed response is
// available for replay for the full configured retention window.
func (s *RedisStore) Commit(ctx context.Context, key string, e Entry, ttl time.Duration) error {
	data, err := marshalEntry(e)
	if err != nil {
		return fmt.Errorf("idempotency: marshal committed entry: %w", err)
	}
	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("idempotency: commit %q: %w", key, err)
	}
	return nil
}

// Release removes the InFlight reservation for key so the client may retry
// with the same key after resolving the underlying problem.
func (s *RedisStore) Release(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("idempotency: release %q: %w", key, err)
	}
	return nil
}

var _ Store = (*RedisStore)(nil)

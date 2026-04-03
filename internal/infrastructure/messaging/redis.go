package messaging

// TODO: implement RedisPublisher and RedisSubscriber using go-redis/v9's
// PubSub support. The Redis client is provided at construction time (injected
// from the composition root) rather than created internally, so that the
// same connection pool is shared with the cache layer without duplication.

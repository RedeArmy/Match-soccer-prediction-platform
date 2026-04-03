// Package cache provides the Redis client used for caching and ephemeral
// data storage across the application.
//
// Redis serves two distinct purposes in this service:
//   1. Response caching — reducing repeated database reads for frequently
//      accessed, slowly changing data such as the published match schedule.
//   2. Pub/sub messaging — delivering domain events between the API server
//      and the worker process when in-order, at-least-once delivery is
//      acceptable and the overhead of a dedicated broker is not justified.
//
// If these two concerns grow sufficiently complex, consider separating them
// into distinct packages with their own connection pools.
package cache

// TODO: implement a Redis client constructor analogous to database.NewPool.
// Accept a config.RedisConfig, construct a *redis.Client (go-redis/v9),
// verify connectivity via Ping, and return the client or a descriptive error.

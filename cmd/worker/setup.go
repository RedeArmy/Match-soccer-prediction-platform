package main

// setup.go contains the infrastructure bootstrap helpers used by the worker's
// main function. Each function constructs exactly one dependency and returns
// a cleanup function where applicable, keeping main.go focused on lifecycle
// management rather than construction details.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// setupDB opens a connection pool to the primary PostgreSQL database.
//
// Unlike the API server's setupDB, this function intentionally does NOT run
// database migrations. Migrations are applied exclusively by the API server,
// which holds a PostgreSQL advisory lock during the operation. If the worker
// also attempted migrations, the two processes would contend for the lock on
// startup, causing one to fail. The worker must therefore assume that the
// schema is already up to date before it starts.
func setupDB(ctx context.Context, cfg *config.Config, log *zap.Logger) (*pgxpool.Pool, error) {
	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("database DSN is required (WCQ_DATABASE_DSN)")
	}

	pool, err := database.NewPool(ctx, database.Config{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("database unavailable: %w", err)
	}
	log.Sugar().Info("database connection established")
	return pool, nil
}

// setupEventBus constructs the Redis-backed event bus required by the worker.
//
// The worker only functions correctly with the Redis driver. In-memory events
// are not visible across process boundaries, so an in-memory bus would never
// receive the MatchFinished events published by the API server. Rather than
// silently doing nothing, this function fails fast with an actionable error
// message when the driver is not "redis".
func setupEventBus(ctx context.Context, cfg *config.Config, log *zap.Logger) (events.Bus, func(), error) {
	if cfg.EventBus.Driver != "redis" {
		return nil, func() {}, fmt.Errorf(
			"worker requires eventBus.driver=redis, got %q; set WCQ_EVENTBUS_DRIVER=redis",
			cfg.EventBus.Driver,
		)
	}

	redisClient, err := cache.NewClient(ctx, cache.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("redis bus: connect: %w", err)
	}
	log.Sugar().Infof("event bus: using Redis driver (%s)", cfg.Redis.Addr)

	bus := messaging.NewRedisBus(redisClient, log)

	cleanup := func() {
		bus.Close()
		if err := redisClient.Close(); err != nil {
			log.Sugar().Warnf("redis bus: close client: %v", err)
		}
	}
	return bus, cleanup, nil
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/dsem"
)

// snapshotRetryBase is the initial backoff duration between snapshot attempts.
// Subsequent attempts double this value. Declared as a var so tests can set it
// to zero to avoid sleeping.
var snapshotRetryBase = 100 * time.Millisecond

// maxSnapshotAttempts is the maximum snapshot write attempts per quiniela per
// match event. Declared as a var so tests can set it to 1 for deterministic
// behaviour and so worker/main.go can override it from system_params at startup.
var maxSnapshotAttempts = 3

// snapshotConcurrency caps the number of quinielas snapshotted in parallel
// within a single postScoringWork call. Raising the value reduces wall-clock
// latency for large quiniela counts at the cost of more concurrent DB connections.
// Declared as a var so tests can set it to 1 for deterministic ordering.
var snapshotConcurrency = 16

// snapshotSem bounds the total number of concurrent snapshot DB operations
// across ALL concurrent MatchFinished events on this process (and, when the
// distributed implementation is wired, across ALL replicas).
//
// Without a global guard, N simultaneous match finishes each launch their own
// errgroup with snapshotConcurrency goroutines, producing N×concurrency total
// concurrent snapshot queries — enough to saturate the connection pool during
// elimination phases.
//
// Initialised by main.go after reading snapshotConcurrency from system_params.
// When Redis is available, main.go wires a dsem.RedisSemaphore to enforce the
// limit cluster-wide; otherwise it wires a localSnapshotSem.
// nil before initialisation (tests that set snapshotConcurrency=1 bypass the nil guard).
var snapshotSem dsem.Semaphore

// SnapshotLocker prevents duplicate leaderboard snapshot DB writes across
// multiple worker replicas for the same (matchID, quinielaID) pair.
//
// Without a distributed lock, N replicas each running M concurrent snapshot
// goroutines produce N×M concurrent DB writes for the same data.  With a lock,
// the first replica to acquire it does the work; the others skip that pair.
//
// The lock must be best-effort: a Redis failure should degrade to the single-
// process semaphore behaviour, not halt snapshot generation entirely.
type SnapshotLocker interface {
	// TryLock attempts to acquire an exclusive snapshot slot for the given pair.
	// Returns (true, nil) when the lock was acquired — this replica must run the
	// snapshot. Returns (false, nil) when another replica holds the lock — skip.
	// Returns (_, non-nil) on transient infrastructure failure — caller should
	// proceed without the lock (degrade gracefully).
	TryLock(ctx context.Context, matchID, quinielaID int) (bool, error)
	// Unlock releases the lock after the snapshot completes.  Errors are
	// logged by the caller but not propagated: the TTL on the Redis key acts as
	// a safety net if Unlock is never reached (process crash, context cancel).
	Unlock(ctx context.Context, matchID, quinielaID int) error
}

// noopSnapshotLocker always grants the lock.  Used in tests and as a fallback
// when Redis is unavailable, preserving the in-process semaphore behaviour.
type noopSnapshotLocker struct{}

func (noopSnapshotLocker) TryLock(_ context.Context, _, _ int) (bool, error) { return true, nil }
func (noopSnapshotLocker) Unlock(_ context.Context, _, _ int) error          { return nil }

// decodePayload re-encodes env.Payload as JSON and then unmarshals it into T.
//
// events.Envelope.Payload is declared as `any`. When InMemoryBus delivers an
// event the concrete Go type (e.g. events.MatchFinished) is preserved in
// memory, so a direct type assertion works. RedisBus, however, serialises the
// envelope to JSON before storing it in a Stream entry and deserialises it on
// the consumer side; any field typed `any` becomes map[string]interface{}
// after the round-trip, which makes the direct assertion fail silently.
//
// The double marshal/unmarshal here is the correct solution for this
// structural mismatch: it is explicit, allocation-bounded, and readable.
func decodePayload[T any](env events.Envelope) (T, error) {
	var result T
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return result, fmt.Errorf("marshal payload: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("unmarshal payload: %w", err)
	}
	return result, nil
}

// newMatchStartedHandler returns the event handler the worker registers for
// EventMatchStarted events. Its responsibility is to emit a structured audit
// log entry so the operations team has a reliable record of when each match
// transitioned to Live status.
//
// Prediction-window enforcement is handled synchronously in PredictionService:
// Submit and Update both check match.Status and reject requests against a Live
// or Finished match. The async event is therefore used exclusively for
// observability, not for enforcement - making the handler safe to implement as
// a fire-and-forget log with no downstream side-effects and no retriable errors.
//
// On a malformed payload the handler logs an error and returns nil.
// Returning nil prevents the bus from retrying a message that can never
// succeed, mirroring the pattern used by newMatchFinishedHandler.
func newMatchStartedHandler(log *zap.Logger) func(context.Context, events.Envelope) error {
	return func(ctx context.Context, env events.Envelope) error {
		ms, err := decodePayload[events.MatchStarted](env)
		if err != nil {
			log.Error("worker: cannot decode MatchStarted payload",
				zap.String("event_type", string(env.Type)),
				zap.Error(err),
			)
			return nil
		}

		log.Info("worker: match started - prediction window closed",
			zap.Int("match_id", ms.MatchID),
			zap.String("home_team", ms.HomeTeam),
			zap.String("away_team", ms.AwayTeam),
			zap.String("kickoff_at", ms.KickoffAt.UTC().Format(time.RFC3339)),
		)
		return nil
	}
}

// newMatchFinishedHandler returns the event handler that the worker registers
// on the bus for EventMatchFinished events. It is extracted as a constructor
// so it can be unit-tested in isolation without starting the full worker.
//
// After scoring succeeds the handler runs postScoringWork, which fetches the
// affected quinielas once, flushes their cache entries via every registered
// PostScoringInvalidator, broadcasts a leaderboard.updated SSE signal to every
// active group member, and then triggers a leaderboard snapshot for each.
// All post-scoring steps are best-effort: failures are logged and swallowed
// because scoring has already committed.
func newMatchFinishedHandler(
	scorer service.MatchScorer,
	snapshotter service.Snapshotter,
	predRepo repository.PredictionRepository,
	invalidators []service.PostScoringInvalidator,
	broadcaster LeaderboardBroadcaster,
	locker SnapshotLocker,
	log *zap.Logger,
) func(context.Context, events.Envelope) error {
	return func(ctx context.Context, env events.Envelope) error {
		mf, err := decodePayload[events.MatchFinished](env)
		if err != nil {
			// A payload that cannot be decoded will never succeed on retry.
			// Log, then return nil so the bus does not route it to the DLQ.
			log.Error("worker: cannot decode MatchFinished payload",
				zap.String("event_type", string(env.Type)),
				zap.Error(err),
			)
			return nil
		}

		if err := scorer.ScoreMatch(ctx, mf.MatchID); err != nil {
			log.Error("worker: scoring failed after MatchFinished event",
				zap.Int("match_id", mf.MatchID),
				zap.Error(err),
			)
			// Return the error so the bus retries and, if all attempts fail,
			// pushes the event to the dead-letter queue for manual replay.
			return fmt.Errorf("score match %d: %w", mf.MatchID, err)
		}

		log.Sugar().Infof("worker: scored match %d (%s %d-%d %s)",
			mf.MatchID, mf.HomeTeam, mf.HomeScore, mf.AwayScore, mf.AwayTeam)

		postScoringWork(ctx, mf.MatchID, snapshotter, predRepo, invalidators, broadcaster, locker, log)
		return nil
	}
}

// postScoringWork fetches the quinielas affected by matchID exactly once, then
// runs three steps in this fixed order:
//
//  1. Cache invalidation — flush stale leaderboard entries before any client
//     refetch can arrive.
//  2. SSE broadcast — publish a leaderboard.updated signal to every active
//     group member so connected clients know to refetch immediately.  The
//     signal is sent after invalidation so the cache is already cold when the
//     refetch request arrives at the API server.
//  3. Snapshot — write the post-scoring standings to the snapshot table for
//     historical queries.  Runs concurrently with the ongoing response to
//     clients' refetches; the snapshot table is separate from the live cache.
//
// All steps are best-effort. A failure to fetch quiniela IDs skips all three
// but does not propagate: scoring has already committed.
func postScoringWork(
	ctx context.Context,
	matchID int,
	snapshotter service.Snapshotter,
	predRepo repository.PredictionRepository,
	invalidators []service.PostScoringInvalidator,
	broadcaster LeaderboardBroadcaster,
	locker SnapshotLocker,
	log *zap.Logger,
) {
	if predRepo == nil {
		return
	}

	quinielaIDs, err := predRepo.ListQuinielaIDsByMatch(ctx, matchID)
	if err != nil {
		log.Warn("worker: could not fetch quiniela IDs after scoring",
			zap.Int("match_id", matchID),
			zap.Error(err),
		)
		return
	}
	if len(quinielaIDs) == 0 {
		return
	}

	// Step 1: flush stale cache entries before broadcasting the refetch signal.
	for _, inv := range invalidators {
		inv.InvalidateAfterScoring(ctx, quinielaIDs)
	}

	// Step 2: signal connected SSE clients to refetch the leaderboard.
	// Must run after invalidation so the cache is cold when the client arrives.
	if broadcaster != nil {
		broadcaster.BroadcastLeaderboardUpdated(ctx, quinielaIDs)
	}

	if snapshotter == nil {
		return
	}

	// Fan-out snapshot generation across quinielas with a bounded goroutine pool.
	// g.SetLimit bounds goroutine creation per-event (memory efficiency); the
	// global snapshotSem bounds total concurrent DB operations across ALL events
	// on this machine; locker prevents duplicate work across replicas.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(snapshotConcurrency)
	for _, qid := range quinielaIDs {
		qid := qid
		g.Go(func() error {
			if snapshotSem != nil {
				if err := snapshotSem.Acquire(gctx); err != nil {
					return nil // context cancelled; scoring already committed
				}
				defer snapshotSem.Release()
			}
			runSnapshot(gctx, matchID, qid, snapshotter, locker, log)
			return nil
		})
	}
	_ = g.Wait()
}

// runSnapshot acquires the distributed lock for this (matchID, quinielaID) pair,
// runs the snapshot with retries, and releases the lock.  If the lock is held by
// another replica the work is skipped.  A lock acquisition failure degrades to
// single-machine behaviour without halting snapshot generation.
func runSnapshot(
	ctx context.Context,
	matchID, quinielaID int,
	snapshotter service.Snapshotter,
	locker SnapshotLocker,
	log *zap.Logger,
) {
	if locker != nil {
		ok, err := locker.TryLock(ctx, matchID, quinielaID)
		switch {
		case err != nil:
			log.Warn("worker: snapshot lock unavailable, proceeding without distributed lock",
				zap.Int("match_id", matchID),
				zap.Int("quiniela_id", quinielaID),
				zap.Error(err),
			)
		case !ok:
			log.Debug("worker: snapshot already claimed by another replica, skipping",
				zap.Int("match_id", matchID),
				zap.Int("quiniela_id", quinielaID),
			)
			return
		default:
			defer func() {
				// context.WithoutCancel so the unlock survives ctx cancellation.
				if err := locker.Unlock(context.WithoutCancel(ctx), matchID, quinielaID); err != nil {
					log.Warn("worker: snapshot unlock failed",
						zap.Int("match_id", matchID),
						zap.Int("quiniela_id", quinielaID),
						zap.Error(err),
					)
				}
			}()
		}
	}
	retrySnapshot(ctx, matchID, quinielaID, snapshotter, log)
}

// retrySnapshot attempts to take a leaderboard snapshot for quinielaID up to
// maxSnapshotAttempts times with exponential backoff starting at
// snapshotRetryBase. On final failure the error is logged at Warn level and
// the function returns without propagating it: scoring has already committed
// and the snapshot is regenerated on the next event.
func retrySnapshot(
	ctx context.Context,
	matchID int,
	quinielaID int,
	snapshotter service.Snapshotter,
	log *zap.Logger,
) {
	backoff := snapshotRetryBase
	for attempt := 1; attempt <= maxSnapshotAttempts; attempt++ {
		if _, err := snapshotter.SnapshotForMatch(ctx, quinielaID, matchID); err == nil {
			log.Sugar().Infof("worker: leaderboard snapshot saved for quiniela %d (match %d)", quinielaID, matchID)
			return
		} else if attempt == maxSnapshotAttempts {
			log.Warn("worker: leaderboard snapshot failed after all retries",
				zap.Int("match_id", matchID),
				zap.Int("quiniela_id", quinielaID),
				zap.Int("attempts", attempt),
				zap.Error(err),
			)
			return
		} else {
			log.Warn("worker: leaderboard snapshot failed, retrying",
				zap.Int("match_id", matchID),
				zap.Int("quiniela_id", quinielaID),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.Error(err),
			)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

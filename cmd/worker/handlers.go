package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// snapshotRetryBase is the initial backoff duration between snapshot attempts.
// Subsequent attempts double this value. Declared as a var so tests can set it
// to zero to avoid sleeping.
var snapshotRetryBase = 100 * time.Millisecond

const maxSnapshotAttempts = 3

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
// After scoring succeeds, the handler triggers a leaderboard snapshot for
// every quiniela that has active, paid members with predictions on the finished
// match. Snapshot failures are logged and swallowed: scoring already committed
// and the snapshot can be regenerated on the next scoring event or manually.
func newMatchFinishedHandler(
	scorer service.MatchScorer,
	snapshotter service.Snapshotter,
	predRepo repository.PredictionRepository,
	log *zap.Logger,
) func(context.Context, events.Envelope) error {
	return func(ctx context.Context, env events.Envelope) error {
		mf, err := decodePayload[events.MatchFinished](env)
		if err != nil {
			// A payload that cannot be decoded will never succeed on retry.
			// Log, then return nil so the bus does not route it to the DLQ -
			// retrying a structurally invalid message would burn retry budget.
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
			return err
		}

		log.Sugar().Infof("worker: scored match %d (%s %d-%d %s)",
			mf.MatchID, mf.HomeTeam, mf.HomeScore, mf.AwayScore, mf.AwayTeam)

		snapshotAffectedQuinielas(ctx, mf.MatchID, snapshotter, predRepo, log)
		return nil
	}
}

// snapshotAffectedQuinielas queries which quinielas had active paid members
// with predictions on matchID, then persists a fresh leaderboard snapshot for
// each using retrySnapshot. Errors after all retries are logged and swallowed:
// scoring has already committed and the snapshot can be regenerated on the next
// scoring event or via manual replay.
//
// When multiple matches finish simultaneously and their workers overlap,
// concurrent calls to Snapshot for the same quiniela produce redundant rows.
// This is safe: GetLatest always returns the most recent snapshot, which
// reflects all committed scores by the time it is taken.
func snapshotAffectedQuinielas(
	ctx context.Context,
	matchID int,
	snapshotter service.Snapshotter,
	predRepo repository.PredictionRepository,
	log *zap.Logger,
) {
	if predRepo == nil || snapshotter == nil {
		return
	}
	quinielaIDs, err := predRepo.ListQuinielaIDsByMatch(ctx, matchID)
	if err != nil {
		log.Warn("worker: could not fetch quiniela IDs for snapshot after scoring",
			zap.Int("match_id", matchID),
			zap.Error(err),
		)
		return
	}

	for _, qid := range quinielaIDs {
		retrySnapshot(ctx, matchID, qid, snapshotter, log)
	}
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
		if _, err := snapshotter.Snapshot(ctx, quinielaID); err == nil {
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

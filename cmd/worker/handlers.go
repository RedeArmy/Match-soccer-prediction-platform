package main

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/service"
)

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

// newMatchFinishedHandler returns the event handler that the worker registers
// on the bus for EventMatchFinished events. It is extracted as a constructor
// so it can be unit-tested in isolation without starting the full worker.
func newMatchFinishedHandler(scorer service.MatchScorer, log *zap.Logger) func(context.Context, events.Envelope) error {
	return func(ctx context.Context, env events.Envelope) error {
		mf, err := decodePayload[events.MatchFinished](env)
		if err != nil {
			// A payload that cannot be decoded will never succeed on retry.
			// Log, then return nil so the bus does not route it to the DLQ —
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

		log.Sugar().Infof("worker: scored match %d (%s %d–%d %s)",
			mf.MatchID, mf.HomeTeam, mf.HomeScore, mf.AwayScore, mf.AwayTeam)
		return nil
	}
}

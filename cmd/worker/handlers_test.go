package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

// stubScorer is a MatchScorer stub that records the last matchID it received
// and can be configured to return an error.
type stubScorer struct {
	called int
	lastID int
	err    error
}

func (s *stubScorer) ScoreMatch(_ context.Context, matchID int) error {
	s.called++
	s.lastID = matchID
	return s.err
}

// envelopeWithMap simulates the payload state produced by RedisBus after a
// JSON round-trip: the concrete events.MatchFinished struct becomes a
// map[string]interface{} because encoding/json unmarshals `any` fields that way.
func envelopeWithMap(matchID int) events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
		Payload: map[string]interface{}{
			"MatchID":   float64(matchID), // JSON numbers unmarshal as float64
			"HomeTeam":  teamMexico,
			"AwayTeam":  "Canada",
			"HomeScore": float64(2),
			"AwayScore": float64(1),
		},
	}
}

// envelopeWithStruct simulates the payload state produced by InMemoryBus:
// the concrete type is preserved in memory, so a type assertion would work
// without decodePayload. decodePayload must also handle this case correctly.
func envelopeWithStruct(matchID int) events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
		Payload: events.MatchFinished{
			MatchID:   matchID,
			HomeTeam:  teamMexico,
			AwayTeam:  "Canada",
			HomeScore: 2,
			AwayScore: 1,
		},
	}
}

// ── decodePayload ─────────────────────────────────────────────────────────────

func TestDecodePayload_MapPayload_DecodesCorrectly(t *testing.T) {
	env := envelopeWithMap(42)
	got, err := decodePayload[events.MatchFinished](env)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.MatchID != 42 {
		t.Errorf("expected MatchID=42, got %d", got.MatchID)
	}
	if got.HomeTeam != teamMexico {
		t.Errorf("expected HomeTeam=%s, got %q", teamMexico, got.HomeTeam)
	}
}

func TestDecodePayload_StructPayload_DecodesCorrectly(t *testing.T) {
	env := envelopeWithStruct(7)
	got, err := decodePayload[events.MatchFinished](env)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.MatchID != 7 {
		t.Errorf("expected MatchID=7, got %d", got.MatchID)
	}
}

func TestDecodePayload_UnmarshalablePayload_ReturnsError(t *testing.T) {
	// A channel cannot be marshalled to JSON, so Marshal will fail.
	env := events.Envelope{
		Type:    events.EventMatchFinished,
		Payload: make(chan int),
	}
	_, err := decodePayload[events.MatchFinished](env)
	if err == nil {
		t.Error("expected error for unmarshalable payload, got nil")
	}
}

func TestDecodePayload_WrongTypeInMap_ReturnsError(t *testing.T) {
	// MatchID is int; "not-a-number" cannot be decoded into int → unmarshal fails.
	env := events.Envelope{
		Type: events.EventMatchFinished,
		Payload: map[string]interface{}{
			"MatchID": "not-a-number",
		},
	}
	_, err := decodePayload[events.MatchFinished](env)
	if err == nil {
		t.Error("expected unmarshal error for wrong type, got nil")
	}
}

// ── newMatchFinishedHandler ───────────────────────────────────────────────────

func TestMatchFinishedHandler_MapPayload_CallsScorer(t *testing.T) {
	scorer := &stubScorer{}
	h := newMatchFinishedHandler(scorer, zap.NewNop())

	if err := h(context.Background(), envelopeWithMap(99)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 {
		t.Errorf("expected ScoreMatch called once, got %d", scorer.called)
	}
	if scorer.lastID != 99 {
		t.Errorf("expected ScoreMatch(99), got ScoreMatch(%d)", scorer.lastID)
	}
}

func TestMatchFinishedHandler_StructPayload_CallsScorer(t *testing.T) {
	scorer := &stubScorer{}
	h := newMatchFinishedHandler(scorer, zap.NewNop())

	if err := h(context.Background(), envelopeWithStruct(5)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 || scorer.lastID != 5 {
		t.Errorf("expected ScoreMatch(5), got called=%d id=%d", scorer.called, scorer.lastID)
	}
}

func TestMatchFinishedHandler_ScorerError_PropagatesError(t *testing.T) {
	scorer := &stubScorer{err: errors.New("db down")}
	h := newMatchFinishedHandler(scorer, zap.NewNop())

	err := h(context.Background(), envelopeWithMap(1))
	if err == nil {
		t.Error("expected error from scorer to be returned, got nil")
	}
}

func TestMatchFinishedHandler_UndecodablePayload_ReturnsNil(t *testing.T) {
	// A channel payload cannot be JSON-marshalled. The handler must log and
	// return nil rather than propagating the error, because retrying a
	// structurally invalid message would burn retry budget uselessly.
	scorer := &stubScorer{}
	h := newMatchFinishedHandler(scorer, zap.NewNop())

	env := events.Envelope{
		Type:    events.EventMatchFinished,
		Payload: make(chan int),
	}
	if err := h(context.Background(), env); err != nil {
		t.Errorf("expected nil for undecodable payload, got %v", err)
	}
	if scorer.called != 0 {
		t.Error("scorer should not have been called for undecodable payload")
	}
}

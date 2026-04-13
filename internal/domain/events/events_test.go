// Package events_test exercises the public surface of the events package.
//
// Each Validate method is tested in isolation: one "happy path" test confirms
// that a fully populated value returns nil, and one test per invariant confirms
// that violating that invariant returns an apperrors.ErrValidation error.
// Tests are black-box (package events_test) so they only depend on the
// exported API and remain valid even if the internal representation changes.
package events_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// isValidation is a helper that reports whether err is an apperrors validation
// error, mirroring the pattern used across the domain test suite.
func isValidation(err error) bool {
	return errors.Is(err, apperrors.ErrValidation)
}

const (
	teamBrazil    = "Brazil"
	teamArgentina = "Argentina"

	errZeroMatchID = "expected validation error for zero MatchID"
	errNegMatchID  = "expected validation error for negative MatchID"
)

// ── Envelope ──────────────────────────────────────────────────────────────────

func TestEnvelope_Valid_ReturnsNil(t *testing.T) {
	env := events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
		Payload:    events.MatchFinished{MatchID: 1},
	}
	if err := env.Validate(); err != nil {
		t.Errorf("expected nil for valid envelope, got %v", err)
	}
}

func TestEnvelope_EmptyType_ReturnsValidation(t *testing.T) {
	env := events.Envelope{
		OccurredAt: time.Now(),
		Payload:    events.MatchFinished{MatchID: 1},
	}
	if !isValidation(env.Validate()) {
		t.Error("expected validation error for empty Type")
	}
}

func TestEnvelope_ZeroOccurredAt_ReturnsValidation(t *testing.T) {
	env := events.Envelope{
		Type:    events.EventMatchFinished,
		Payload: events.MatchFinished{MatchID: 1},
	}
	if !isValidation(env.Validate()) {
		t.Error("expected validation error for zero OccurredAt")
	}
}

func TestEnvelope_NilPayload_ReturnsValidation(t *testing.T) {
	env := events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
	}
	if !isValidation(env.Validate()) {
		t.Error("expected validation error for nil Payload")
	}
}

// ── MatchStarted ──────────────────────────────────────────────────────────────

func TestMatchStarted_Valid_ReturnsNil(t *testing.T) {
	e := events.MatchStarted{
		MatchID:   1,
		HomeTeam:  teamBrazil,
		AwayTeam:  teamArgentina,
		KickoffAt: time.Now().Add(time.Hour),
	}
	if err := e.Validate(); err != nil {
		t.Errorf("expected nil for valid MatchStarted, got %v", err)
	}
}

func TestMatchStarted_ZeroMatchID_ReturnsValidation(t *testing.T) {
	e := events.MatchStarted{HomeTeam: teamBrazil, AwayTeam: teamArgentina, KickoffAt: time.Now().Add(time.Hour)}
	if !isValidation(e.Validate()) {
		t.Error(errZeroMatchID)
	}
}

func TestMatchStarted_NegativeMatchID_ReturnsValidation(t *testing.T) {
	e := events.MatchStarted{MatchID: -1, HomeTeam: teamBrazil, AwayTeam: teamArgentina, KickoffAt: time.Now().Add(time.Hour)}
	if !isValidation(e.Validate()) {
		t.Error(errNegMatchID)
	}
}

func TestMatchStarted_EmptyHomeTeam_ReturnsValidation(t *testing.T) {
	e := events.MatchStarted{MatchID: 1, AwayTeam: teamArgentina, KickoffAt: time.Now().Add(time.Hour)}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for empty HomeTeam")
	}
}

func TestMatchStarted_EmptyAwayTeam_ReturnsValidation(t *testing.T) {
	e := events.MatchStarted{MatchID: 1, HomeTeam: teamBrazil, KickoffAt: time.Now().Add(time.Hour)}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for empty AwayTeam")
	}
}

func TestMatchStarted_ZeroKickoffAt_ReturnsValidation(t *testing.T) {
	e := events.MatchStarted{MatchID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for zero KickoffAt")
	}
}

// ── MatchFinished ─────────────────────────────────────────────────────────────

func TestMatchFinished_Valid_ReturnsNil(t *testing.T) {
	e := events.MatchFinished{
		MatchID:   1,
		HomeTeam:  teamBrazil,
		AwayTeam:  teamArgentina,
		HomeScore: 2,
		AwayScore: 1,
	}
	if err := e.Validate(); err != nil {
		t.Errorf("expected nil for valid MatchFinished, got %v", err)
	}
}

func TestMatchFinished_ZeroZeroScore_ReturnsNil(t *testing.T) {
	e := events.MatchFinished{MatchID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina}
	if err := e.Validate(); err != nil {
		t.Errorf("expected nil for 0-0 result, got %v", err)
	}
}

func TestMatchFinished_ZeroMatchID_ReturnsValidation(t *testing.T) {
	e := events.MatchFinished{HomeTeam: teamBrazil, AwayTeam: teamArgentina}
	if !isValidation(e.Validate()) {
		t.Error(errZeroMatchID)
	}
}

func TestMatchFinished_NegativeMatchID_ReturnsValidation(t *testing.T) {
	e := events.MatchFinished{MatchID: -5, HomeTeam: teamBrazil, AwayTeam: teamArgentina}
	if !isValidation(e.Validate()) {
		t.Error(errNegMatchID)
	}
}

func TestMatchFinished_EmptyHomeTeam_ReturnsValidation(t *testing.T) {
	e := events.MatchFinished{MatchID: 1, AwayTeam: teamArgentina}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for empty HomeTeam")
	}
}

func TestMatchFinished_EmptyAwayTeam_ReturnsValidation(t *testing.T) {
	e := events.MatchFinished{MatchID: 1, HomeTeam: teamBrazil}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for empty AwayTeam")
	}
}

func TestMatchFinished_NegativeHomeScore_ReturnsValidation(t *testing.T) {
	e := events.MatchFinished{MatchID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina, HomeScore: -1}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for negative HomeScore")
	}
}

func TestMatchFinished_NegativeAwayScore_ReturnsValidation(t *testing.T) {
	e := events.MatchFinished{MatchID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina, AwayScore: -1}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for negative AwayScore")
	}
}

// ── PredictionMade ────────────────────────────────────────────────────────────

func TestPredictionMade_Valid_ReturnsNil(t *testing.T) {
	e := events.PredictionMade{
		PredictionID: 10,
		UserID:       5,
		MatchID:      3,
		HomeScore:    1,
		AwayScore:    2,
	}
	if err := e.Validate(); err != nil {
		t.Errorf("expected nil for valid PredictionMade, got %v", err)
	}
}

func TestPredictionMade_ZeroPredictionID_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{UserID: 5, MatchID: 3}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for zero PredictionID")
	}
}

func TestPredictionMade_NegativePredictionID_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: -1, UserID: 5, MatchID: 3}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for negative PredictionID")
	}
}

func TestPredictionMade_ZeroUserID_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: 10, MatchID: 3}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for zero UserID")
	}
}

func TestPredictionMade_NegativeUserID_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: 10, UserID: -1, MatchID: 3}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for negative UserID")
	}
}

func TestPredictionMade_ZeroMatchID_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: 10, UserID: 5}
	if !isValidation(e.Validate()) {
		t.Error(errZeroMatchID)
	}
}

func TestPredictionMade_NegativeMatchID_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: 10, UserID: 5, MatchID: -3}
	if !isValidation(e.Validate()) {
		t.Error(errNegMatchID)
	}
}

func TestPredictionMade_NegativeHomeScore_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: 10, UserID: 5, MatchID: 3, HomeScore: -1}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for negative HomeScore")
	}
}

func TestPredictionMade_NegativeAwayScore_ReturnsValidation(t *testing.T) {
	e := events.PredictionMade{PredictionID: 10, UserID: 5, MatchID: 3, AwayScore: -1}
	if !isValidation(e.Validate()) {
		t.Error("expected validation error for negative AwayScore")
	}
}

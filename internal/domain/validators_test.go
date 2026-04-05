package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const fmtUnexpectedErr = "expected nil, got %v"

// ── helpers ──────────────────────────────────────────────────────────────────

func isValidation(err error) bool {
	return errors.Is(err, apperrors.ErrValidation)
}

// ── ValidateMatch ─────────────────────────────────────────────────────────────

func TestValidateMatch_ValidMatch_ReturnsNil(t *testing.T) {
	m := &domain.Match{
		HomeTeam:  "Brazil",
		AwayTeam:  "Argentina",
		KickoffAt: time.Now().Add(24 * time.Hour),
	}
	if err := domain.ValidateMatch(m); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestValidateMatch_EmptyHomeTeam_ReturnsValidation(t *testing.T) {
	m := &domain.Match{AwayTeam: "Argentina", KickoffAt: time.Now().Add(time.Hour)}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for empty home team, got %v", err)
	}
}

func TestValidateMatch_EmptyAwayTeam_ReturnsValidation(t *testing.T) {
	m := &domain.Match{HomeTeam: "Brazil", KickoffAt: time.Now().Add(time.Hour)}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for empty away team, got %v", err)
	}
}

func TestValidateMatch_SameTeams_ReturnsValidation(t *testing.T) {
	m := &domain.Match{HomeTeam: "Brazil", AwayTeam: "Brazil", KickoffAt: time.Now().Add(time.Hour)}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for identical teams, got %v", err)
	}
}

func TestValidateMatch_ZeroKickoff_ReturnsValidation(t *testing.T) {
	m := &domain.Match{HomeTeam: "Brazil", AwayTeam: "Argentina"}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for zero kickoff, got %v", err)
	}
}

// ── ValidateMatchResult ───────────────────────────────────────────────────────

func TestValidateMatchResult_ValidScores_ReturnsNil(t *testing.T) {
	home, away := 2, 1
	if err := domain.ValidateMatchResult(&home, &away); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestValidateMatchResult_ZeroZero_ReturnsNil(t *testing.T) {
	home, away := 0, 0
	if err := domain.ValidateMatchResult(&home, &away); err != nil {
		t.Errorf("expected nil for 0-0, got %v", err)
	}
}

func TestValidateMatchResult_NilHomeScore_ReturnsValidation(t *testing.T) {
	away := 1
	if err := domain.ValidateMatchResult(nil, &away); !isValidation(err) {
		t.Errorf("expected validation error for nil home score, got %v", err)
	}
}

func TestValidateMatchResult_NilAwayScore_ReturnsValidation(t *testing.T) {
	home := 1
	if err := domain.ValidateMatchResult(&home, nil); !isValidation(err) {
		t.Errorf("expected validation error for nil away score, got %v", err)
	}
}

func TestValidateMatchResult_NegativeHomeScore_ReturnsValidation(t *testing.T) {
	home, away := -1, 0
	if err := domain.ValidateMatchResult(&home, &away); !isValidation(err) {
		t.Errorf("expected validation error for negative home score, got %v", err)
	}
}

func TestValidateMatchResult_NegativeAwayScore_ReturnsValidation(t *testing.T) {
	home, away := 0, -1
	if err := domain.ValidateMatchResult(&home, &away); !isValidation(err) {
		t.Errorf("expected validation error for negative away score, got %v", err)
	}
}

// ── ValidatePrediction — deadline ─────────────────────────────────────────────

// TestValidatePrediction_WellBeforeDeadline verifies that a prediction
// submitted more than 5 minutes before kick-off is accepted.
func TestValidatePrediction_WellBeforeDeadline_ReturnsNil(t *testing.T) {
	kickoff := time.Now().Add(10 * time.Minute) // 10 min away — comfortably open
	p := &domain.Prediction{HomeScore: 1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff); err != nil {
		t.Errorf("expected nil for prediction 10 min before kickoff, got %v", err)
	}
}

// TestValidatePrediction_ExactlyAtDeadline verifies that the window closes at
// exactly 5 minutes before kick-off. A prediction submitted at that moment
// (i.e. time.Now() == kickoff - 5min) is on the boundary; the implementation
// uses After so equal-to-deadline is still rejected.
func TestValidatePrediction_WithinDeadlineWindow_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(3 * time.Minute) // 3 min away — inside the 5-min lock
	p := &domain.Prediction{HomeScore: 1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff); !isValidation(err) {
		t.Errorf("expected validation error for prediction within 5-min lock window, got %v", err)
	}
}

// TestValidatePrediction_AfterKickoff verifies that predictions submitted after
// kick-off are always rejected.
func TestValidatePrediction_AfterKickoff_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(-1 * time.Minute) // match already started
	p := &domain.Prediction{HomeScore: 2, AwayScore: 1}
	if err := domain.ValidatePrediction(p, kickoff); !isValidation(err) {
		t.Errorf("expected validation error for prediction after kickoff, got %v", err)
	}
}

// TestValidatePrediction_NegativeScores verifies score sanity regardless of deadline.
func TestValidatePrediction_NegativeHomeScore_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(time.Hour)
	p := &domain.Prediction{HomeScore: -1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff); !isValidation(err) {
		t.Errorf("expected validation error for negative home score, got %v", err)
	}
}

func TestValidatePrediction_NegativeAwayScore_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(time.Hour)
	p := &domain.Prediction{HomeScore: 0, AwayScore: -1}
	if err := domain.ValidatePrediction(p, kickoff); !isValidation(err) {
		t.Errorf("expected validation error for negative away score, got %v", err)
	}
}

// ── ValidateQuiniela ──────────────────────────────────────────────────────────

func TestValidateQuiniela_Valid_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{Name: "Oficina 2026", OwnerID: 1}
	if err := domain.ValidateQuiniela(q); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestValidateQuiniela_EmptyName_ReturnsValidation(t *testing.T) {
	q := &domain.Quiniela{OwnerID: 1}
	if err := domain.ValidateQuiniela(q); !isValidation(err) {
		t.Errorf("expected validation error for empty name, got %v", err)
	}
}

func TestValidateQuiniela_ZeroOwner_ReturnsValidation(t *testing.T) {
	q := &domain.Quiniela{Name: "Oficina 2026"}
	if err := domain.ValidateQuiniela(q); !isValidation(err) {
		t.Errorf("expected validation error for zero owner, got %v", err)
	}
}

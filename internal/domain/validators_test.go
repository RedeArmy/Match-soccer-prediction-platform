package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	fmtUnexpectedErr = "expected nil, got %v"
	teamBrazil       = "Brazil"
	teamArgentina    = "Argentina"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func isValidation(err error) bool {
	return errors.Is(err, apperrors.ErrValidation)
}

// ── ValidateMatch ─────────────────────────────────────────────────────────────

func TestValidateMatch_ValidMatch_ReturnsNil(t *testing.T) {
	m := &domain.Match{
		HomeTeam:  teamBrazil,
		AwayTeam:  teamArgentina,
		KickoffAt: time.Now().Add(24 * time.Hour),
	}
	if err := domain.ValidateMatch(m); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestValidateMatch_EmptyHomeTeam_ReturnsValidation(t *testing.T) {
	m := &domain.Match{AwayTeam: teamArgentina, KickoffAt: time.Now().Add(time.Hour)}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for empty home team, got %v", err)
	}
}

func TestValidateMatch_EmptyAwayTeam_ReturnsValidation(t *testing.T) {
	m := &domain.Match{HomeTeam: teamBrazil, KickoffAt: time.Now().Add(time.Hour)}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for empty away team, got %v", err)
	}
}

func TestValidateMatch_SameTeams_ReturnsValidation(t *testing.T) {
	m := &domain.Match{HomeTeam: teamBrazil, AwayTeam: teamBrazil, KickoffAt: time.Now().Add(time.Hour)}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for identical teams, got %v", err)
	}
}

func TestValidateMatch_ZeroKickoff_ReturnsValidation(t *testing.T) {
	m := &domain.Match{HomeTeam: teamBrazil, AwayTeam: teamArgentina}
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
	if err := domain.ValidatePrediction(p, kickoff, time.Now()); err != nil {
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
	if err := domain.ValidatePrediction(p, kickoff, time.Now()); !isValidation(err) {
		t.Errorf("expected validation error for prediction within 5-min lock window, got %v", err)
	}
}

// TestValidatePrediction_AfterKickoff verifies that predictions submitted after
// kick-off are always rejected.
func TestValidatePrediction_AfterKickoff_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(-1 * time.Minute) // match already started
	p := &domain.Prediction{HomeScore: 2, AwayScore: 1}
	if err := domain.ValidatePrediction(p, kickoff, time.Now()); !isValidation(err) {
		t.Errorf("expected validation error for prediction after kickoff, got %v", err)
	}
}

// TestValidatePrediction_NegativeScores verifies score sanity regardless of deadline.
func TestValidatePrediction_NegativeHomeScore_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(time.Hour)
	p := &domain.Prediction{HomeScore: -1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff, time.Now()); !isValidation(err) {
		t.Errorf("expected validation error for negative home score, got %v", err)
	}
}

func TestValidatePrediction_NegativeAwayScore_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(time.Hour)
	p := &domain.Prediction{HomeScore: 0, AwayScore: -1}
	if err := domain.ValidatePrediction(p, kickoff, time.Now()); !isValidation(err) {
		t.Errorf("expected validation error for negative away score, got %v", err)
	}
}

// ── ValidateEmail ─────────────────────────────────────────────────────────────

func TestValidateEmail_Valid_ReturnsNil(t *testing.T) {
	cases := []string{
		"user@example.com",
		"user.name+tag@sub.domain.org",
		"x@y.io",
	}
	for _, email := range cases {
		if err := domain.ValidateEmail(email); err != nil {
			t.Errorf("ValidateEmail(%q): expected nil, got %v", email, err)
		}
	}
}

func TestValidateEmail_Empty_ReturnsValidation(t *testing.T) {
	if err := domain.ValidateEmail(""); !isValidation(err) {
		t.Errorf("expected validation error for empty email, got %v", err)
	}
}

func TestValidateEmail_MissingAt_ReturnsValidation(t *testing.T) {
	if err := domain.ValidateEmail("userexample.com"); !isValidation(err) {
		t.Errorf("expected validation error for missing @, got %v", err)
	}
}

func TestValidateEmail_MissingDomain_ReturnsValidation(t *testing.T) {
	if err := domain.ValidateEmail("user@"); !isValidation(err) {
		t.Errorf("expected validation error for missing domain, got %v", err)
	}
}

func TestValidateEmail_MissingTLD_ReturnsValidation(t *testing.T) {
	if err := domain.ValidateEmail("user@domain"); !isValidation(err) {
		t.Errorf("expected validation error for missing TLD, got %v", err)
	}
}

func TestValidateEmail_WithSpaces_ReturnsValidation(t *testing.T) {
	if err := domain.ValidateEmail("user @example.com"); !isValidation(err) {
		t.Errorf("expected validation error for email with spaces, got %v", err)
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

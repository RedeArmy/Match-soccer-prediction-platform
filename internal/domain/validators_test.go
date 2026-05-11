package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	fmtUnexpectedErr = "expected nil, got %v"
	teamBrazil       = "Brazil"
	teamArgentina    = "Argentina"
	testGroupName    = "Oficina 2026"
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

// ── ValidateMatch – group_label / phase coherence ────────────────────────────

func groupLabel(s string) *string { return &s }

func TestValidateMatch_GroupStageWithValidLabel_ReturnsNil(t *testing.T) {
	for _, label := range []string{"A", "F", "L"} {
		m := &domain.Match{
			HomeTeam:   teamBrazil,
			AwayTeam:   teamArgentina,
			Phase:      domain.PhaseGroupStage,
			GroupLabel: groupLabel(label),
			KickoffAt:  time.Now().Add(time.Hour),
		}
		if err := domain.ValidateMatch(m); err != nil {
			t.Errorf("label %q: expected nil, got %v", label, err)
		}
	}
}

func TestValidateMatch_GroupStageWithoutLabel_ReturnsValidation(t *testing.T) {
	m := &domain.Match{
		HomeTeam:  teamBrazil,
		AwayTeam:  teamArgentina,
		Phase:     domain.PhaseGroupStage,
		KickoffAt: time.Now().Add(time.Hour),
	}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for missing group_label, got %v", err)
	}
}

func TestValidateMatch_GroupStageWithInvalidLabel_ReturnsValidation(t *testing.T) {
	for _, bad := range []string{"a", "group_a", "Group A", "M", "Z", "1"} {
		m := &domain.Match{
			HomeTeam:   teamBrazil,
			AwayTeam:   teamArgentina,
			Phase:      domain.PhaseGroupStage,
			GroupLabel: groupLabel(bad),
			KickoffAt:  time.Now().Add(time.Hour),
		}
		if err := domain.ValidateMatch(m); !isValidation(err) {
			t.Errorf("label %q: expected validation error, got %v", bad, err)
		}
	}
}

func TestValidateMatch_KnockoutWithLabel_ReturnsValidation(t *testing.T) {
	for _, phase := range []domain.MatchPhase{
		domain.PhaseRoundOf32,
		domain.PhaseRoundOf16,
		domain.PhaseQuarterFinal,
		domain.PhaseSemiFinal,
		domain.PhaseThirdPlace,
		domain.PhaseFinal,
	} {
		m := &domain.Match{
			HomeTeam:   teamBrazil,
			AwayTeam:   teamArgentina,
			Phase:      phase,
			GroupLabel: groupLabel("A"),
			KickoffAt:  time.Now().Add(time.Hour),
		}
		if err := domain.ValidateMatch(m); !isValidation(err) {
			t.Errorf("phase %q: expected validation error for unexpected group_label, got %v", phase, err)
		}
	}
}

func TestValidateMatch_KnockoutWithoutLabel_ReturnsNil(t *testing.T) {
	m := &domain.Match{
		HomeTeam:  teamBrazil,
		AwayTeam:  teamArgentina,
		Phase:     domain.PhaseFinal,
		KickoffAt: time.Now().Add(time.Hour),
	}
	if err := domain.ValidateMatch(m); err != nil {
		t.Errorf("expected nil for knockout match without label, got %v", err)
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

// ── ValidatePrediction - deadline ─────────────────────────────────────────────

// TestValidatePrediction_WellBeforeDeadline verifies that a prediction
// submitted more than 5 minutes before kick-off is accepted.
func TestValidatePrediction_WellBeforeDeadline_ReturnsNil(t *testing.T) {
	kickoff := time.Now().Add(10 * time.Minute) // 10 min away - comfortably open
	p := &domain.Prediction{HomeScore: 1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff, time.Now(), domain.PredictionDeadlineOffset); err != nil {
		t.Errorf("expected nil for prediction 10 min before kickoff, got %v", err)
	}
}

// TestValidatePrediction_ExactlyAtDeadline verifies that the window closes at
// exactly 5 minutes before kick-off. A prediction submitted at that moment
// (i.e. time.Now() == kickoff - 5min) is on the boundary; the implementation
// uses After so equal-to-deadline is still rejected.
func TestValidatePrediction_WithinDeadlineWindow_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(3 * time.Minute) // 3 min away - inside the 5-min lock
	p := &domain.Prediction{HomeScore: 1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff, time.Now(), domain.PredictionDeadlineOffset); !isValidation(err) {
		t.Errorf("expected validation error for prediction within 5-min lock window, got %v", err)
	}
}

// TestValidatePrediction_AfterKickoff verifies that predictions submitted after
// kick-off are always rejected.
func TestValidatePrediction_AfterKickoff_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(-1 * time.Minute) // match already started
	p := &domain.Prediction{HomeScore: 2, AwayScore: 1}
	if err := domain.ValidatePrediction(p, kickoff, time.Now(), domain.PredictionDeadlineOffset); !isValidation(err) {
		t.Errorf("expected validation error for prediction after kickoff, got %v", err)
	}
}

// TestValidatePrediction_NegativeScores verifies score sanity regardless of deadline.
func TestValidatePrediction_NegativeHomeScore_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(time.Hour)
	p := &domain.Prediction{HomeScore: -1, AwayScore: 0}
	if err := domain.ValidatePrediction(p, kickoff, time.Now(), domain.PredictionDeadlineOffset); !isValidation(err) {
		t.Errorf("expected validation error for negative home score, got %v", err)
	}
}

func TestValidatePrediction_NegativeAwayScore_ReturnsValidation(t *testing.T) {
	kickoff := time.Now().Add(time.Hour)
	p := &domain.Prediction{HomeScore: 0, AwayScore: -1}
	if err := domain.ValidatePrediction(p, kickoff, time.Now(), domain.PredictionDeadlineOffset); !isValidation(err) {
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
	q := &domain.Quiniela{Name: testGroupName, OwnerID: 1, EntryFee: 0}
	if err := domain.ValidateQuiniela(q); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestValidateQuiniela_ValidWithEntryFee_ReturnsNil(t *testing.T) {
	q := &domain.Quiniela{Name: testGroupName, OwnerID: 1, EntryFee: 100}
	if err := domain.ValidateQuiniela(q); err != nil {
		t.Errorf("expected nil for valid quiniela with entry fee, got %v", err)
	}
}

func TestValidateQuiniela_EmptyName_ReturnsValidation(t *testing.T) {
	q := &domain.Quiniela{OwnerID: 1}
	if err := domain.ValidateQuiniela(q); !isValidation(err) {
		t.Errorf("expected validation error for empty name, got %v", err)
	}
}

func TestValidateQuiniela_ZeroOwner_ReturnsValidation(t *testing.T) {
	q := &domain.Quiniela{Name: testGroupName}
	if err := domain.ValidateQuiniela(q); !isValidation(err) {
		t.Errorf("expected validation error for zero owner, got %v", err)
	}
}

func TestValidateQuiniela_NegativeEntryFee_ReturnsValidation(t *testing.T) {
	q := &domain.Quiniela{Name: testGroupName, OwnerID: 1, EntryFee: -1}
	if err := domain.ValidateQuiniela(q); !isValidation(err) {
		t.Errorf("expected validation error for negative entry fee, got %v", err)
	}
}

// ── ValidateGroupSize ─────────────────────────────────────────────────────────

func TestValidateGroupSize_AtMinimum_ReturnsNil(t *testing.T) {
	if err := domain.ValidateGroupSize(domain.MinMembersPerGroup); err != nil {
		t.Errorf("expected nil at minimum group size (%d), got %v", domain.MinMembersPerGroup, err)
	}
}

func TestValidateGroupSize_AtMaximum_ReturnsNil(t *testing.T) {
	if err := domain.ValidateGroupSize(domain.MaxMembersPerGroup); err != nil {
		t.Errorf("expected nil at maximum group size (%d), got %v", domain.MaxMembersPerGroup, err)
	}
}

func TestValidateGroupSize_BelowMinimum_ReturnsValidation(t *testing.T) {
	cases := []int{0, 1, 2, 3, 4}
	for _, n := range cases {
		if err := domain.ValidateGroupSize(n); !isValidation(err) {
			t.Errorf("ValidateGroupSize(%d): expected validation error, got %v", n, err)
		}
	}
}

func TestValidateGroupSize_AboveMaximum_ReturnsValidation(t *testing.T) {
	cases := []int{21, 50, 100}
	for _, n := range cases {
		if err := domain.ValidateGroupSize(n); !isValidation(err) {
			t.Errorf("ValidateGroupSize(%d): expected validation error, got %v", n, err)
		}
	}
}

func TestValidateGroupSize_MidRange_ReturnsNil(t *testing.T) {
	cases := []int{6, 9, 10, 14, 15, 19}
	for _, n := range cases {
		if err := domain.ValidateGroupSize(n); err != nil {
			t.Errorf("ValidateGroupSize(%d): expected nil, got %v", n, err)
		}
	}
}

// ── ValidateMatchPhase ────────────────────────────────────────────────────────

func TestValidateMatchPhase_EmptyString_ReturnsNil(t *testing.T) {
	if err := domain.ValidateMatchPhase(""); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestValidateMatchPhase_AllKnownPhases_ReturnNil(t *testing.T) {
	for _, phase := range domain.AllMatchPhases {
		if err := domain.ValidateMatchPhase(phase); err != nil {
			t.Errorf("ValidateMatchPhase(%q): expected nil, got %v", phase, err)
		}
	}
}

func TestValidateMatchPhase_UnknownPhase_ReturnsValidation(t *testing.T) {
	if err := domain.ValidateMatchPhase("semifinals"); !isValidation(err) {
		t.Errorf("expected validation error for unrecognised phase, got %v", err)
	}
}

// ── Length validation tests ───────────────────────────────────────────────────

func TestValidateEmail_ExceedsMaxLength_ReturnsValidation(t *testing.T) {
	// RFC 5321 max is 320. Build a 321-character email that is otherwise valid.
	localPart := strings.Repeat("a", 64)
	// Build domain to push total over 320: 64 + 1(@) + 256 = 321
	domainPart := strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." +
		strings.Repeat("d", 63) + "." + strings.Repeat("e", 63) + ".com"
	oversized := localPart + "@" + domainPart
	if len(oversized) <= 320 {
		t.Fatalf("test setup error: email is %d chars, need >320", len(oversized))
	}
	if err := domain.ValidateEmail(oversized); !isValidation(err) {
		t.Errorf("expected validation error for email >320 chars, got %v", err)
	}
}

func TestValidateEmail_ExactlyAtMaxLength_Accepted(t *testing.T) {
	// 64-char local + @ + 255-char domain = 320 total
	local := strings.Repeat("a", 64)
	// Need exactly 255 chars for domain part to reach RFC 5321 max of 320
	// Structure: 62.62.62.62.com = 62+1+62+1+62+1+62+4 = 255 ✓
	domainPart := strings.Repeat("b", 62) + "." + strings.Repeat("c", 62) + "." +
		strings.Repeat("d", 62) + "." + strings.Repeat("e", 62) + ".com"
	exactly320 := local + "@" + domainPart
	if len(exactly320) != 320 {
		t.Fatalf("test setup error: email is %d chars, expected 320", len(exactly320))
	}
	if err := domain.ValidateEmail(exactly320); err != nil {
		t.Errorf("expected nil for 320-char email, got %v", err)
	}
}

func TestValidateMatch_HomeTeamExceedsMaxLength_ReturnsValidation(t *testing.T) {
	oversized := strings.Repeat("x", 101)
	m := &domain.Match{
		HomeTeam:  oversized,
		AwayTeam:  teamArgentina,
		KickoffAt: time.Now().Add(time.Hour),
	}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for home team >100 chars, got %v", err)
	}
}

func TestValidateMatch_AwayTeamExceedsMaxLength_ReturnsValidation(t *testing.T) {
	oversized := strings.Repeat("y", 101)
	m := &domain.Match{
		HomeTeam:  teamBrazil,
		AwayTeam:  oversized,
		KickoffAt: time.Now().Add(time.Hour),
	}
	if err := domain.ValidateMatch(m); !isValidation(err) {
		t.Errorf("expected validation error for away team >100 chars, got %v", err)
	}
}

func TestValidateMatch_TeamNamesAtMaxLength_Accepted(t *testing.T) {
	exactly100 := strings.Repeat("z", 100)
	m := &domain.Match{
		HomeTeam:  exactly100,
		AwayTeam:  "Short",
		KickoffAt: time.Now().Add(time.Hour),
	}
	if err := domain.ValidateMatch(m); err != nil {
		t.Errorf("expected nil for 100-char team name, got %v", err)
	}
}

func TestValidateQuiniela_NameExceedsMaxLength_ReturnsValidation(t *testing.T) {
	oversized := strings.Repeat("q", 201)
	q := &domain.Quiniela{
		Name:    oversized,
		OwnerID: 1,
	}
	if err := domain.ValidateQuiniela(q); !isValidation(err) {
		t.Errorf("expected validation error for quiniela name >200 chars, got %v", err)
	}
}

func TestValidateQuiniela_NameAtMaxLength_Accepted(t *testing.T) {
	exactly200 := strings.Repeat("n", 200)
	q := &domain.Quiniela{
		Name:    exactly200,
		OwnerID: 1,
	}
	if err := domain.ValidateQuiniela(q); err != nil {
		t.Errorf("expected nil for 200-char quiniela name, got %v", err)
	}
}

func TestValidateUserName_ExceedsMaxLength_ReturnsValidation(t *testing.T) {
	oversized := strings.Repeat("u", 201)
	if err := domain.ValidateUserName(oversized); !isValidation(err) {
		t.Errorf("expected validation error for user name >200 chars, got %v", err)
	}
}

func TestValidateUserName_AtMaxLength_Accepted(t *testing.T) {
	exactly200 := strings.Repeat("w", 200)
	if err := domain.ValidateUserName(exactly200); err != nil {
		t.Errorf("expected nil for 200-char user name, got %v", err)
	}
}

func TestValidateUserName_EmptyString_Accepted(t *testing.T) {
	// Empty names are permitted - Clerk sync falls back to subject ID
	if err := domain.ValidateUserName(""); err != nil {
		t.Errorf("expected nil for empty user name, got %v", err)
	}
}

// ── ParseWinMethod ────────────────────────────────────────────────────────────

func TestParseWinMethod_Normal_ReturnsValue(t *testing.T) {
	wm, err := domain.ParseWinMethod("normal")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if wm != domain.WinMethodNormal {
		t.Errorf("expected WinMethodNormal, got %q", wm)
	}
}

func TestParseWinMethod_ExtraTime_ReturnsValue(t *testing.T) {
	wm, err := domain.ParseWinMethod("extra_time")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if wm != domain.WinMethodExtraTime {
		t.Errorf("expected WinMethodExtraTime, got %q", wm)
	}
}

func TestParseWinMethod_Penalties_ReturnsValue(t *testing.T) {
	wm, err := domain.ParseWinMethod("penalties")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if wm != domain.WinMethodPenalties {
		t.Errorf("expected WinMethodPenalties, got %q", wm)
	}
}

func TestParseWinMethod_UnknownValue_ReturnsValidation(t *testing.T) {
	_, err := domain.ParseWinMethod("overtime")
	if !isValidation(err) {
		t.Errorf("expected validation error for unknown win_method, got %v", err)
	}
}

func TestParseWinMethod_EmptyString_ReturnsValidation(t *testing.T) {
	_, err := domain.ParseWinMethod("")
	if !isValidation(err) {
		t.Errorf("expected validation error for empty win_method, got %v", err)
	}
}

func TestParseWinMethod_CaseSensitive_ReturnsValidation(t *testing.T) {
	_, err := domain.ParseWinMethod("Extra_Time")
	if !isValidation(err) {
		t.Errorf("expected validation error for wrong-cased win_method, got %v", err)
	}
}

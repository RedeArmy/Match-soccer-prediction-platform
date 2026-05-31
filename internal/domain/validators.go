package domain

import (
	"fmt"
	"regexp"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// emailRE is a structural check that catches obvious mistakes (missing "@",
// missing domain, empty local part). Full RFC 5322 compliance is intentionally
// not attempted here - Clerk already validates email format at signup, so this
// check is a defence-in-depth layer, not the primary gate.
var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// validGroupLabels is the canonical set of FIFA World Cup 2026 group letters
// (A–L, 12 groups, 48 teams). Stored as a map for O(1) lookup.
var validGroupLabels = map[string]struct{}{
	"A": {}, "B": {}, "C": {}, "D": {}, "E": {}, "F": {},
	"G": {}, "H": {}, "I": {}, "J": {}, "K": {}, "L": {},
}

// ValidateEmail returns a validation error when email is empty, exceeds the
// RFC 5321 length limit, or fails the basic structural check. Call this wherever
// user email data enters the system (webhook handler, user creation endpoints).
func ValidateEmail(email string) error {
	if email == "" {
		return apperrors.Validation("email must not be empty")
	}
	if len(email) > MaxEmailLength {
		return apperrors.Validation("email must not exceed 320 characters")
	}
	if !emailRE.MatchString(email) {
		return apperrors.Validation("email is not a valid address")
	}
	return nil
}

// ValidateUserName returns a validation error when name exceeds the maximum
// allowed length. Call this before persisting User.Name from webhook payloads
// or admin update endpoints. Empty names are permitted (Clerk sync falls back
// to subject ID when first/last name are both absent).
func ValidateUserName(name string) error {
	if len(name) > MaxNameLength {
		return apperrors.Validation("user name must not exceed 200 characters")
	}
	return nil
}

// ValidateMatch checks that the essential fields of a Match are coherent
// before the entity is persisted for the first time.
//
// This validates business invariants, not HTTP request structure. The handler
// layer is responsible for confirming that the JSON body is well-formed; this
// function confirms that the decoded values make sense for the domain.
func ValidateMatch(m *Match) error {
	if m.HomeTeam == "" {
		return apperrors.Validation("home team must not be empty")
	}
	if len(m.HomeTeam) > MaxTeamNameLength {
		return apperrors.Validation("home team must not exceed 100 characters")
	}
	if m.AwayTeam == "" {
		return apperrors.Validation("away team must not be empty")
	}
	if len(m.AwayTeam) > MaxTeamNameLength {
		return apperrors.Validation("away team must not exceed 100 characters")
	}
	if m.HomeTeam == m.AwayTeam {
		return apperrors.Validation("home team and away team must be different")
	}
	if m.KickoffAt.IsZero() {
		return apperrors.Validation("kick-off time must be set")
	}
	if m.Phase == PhaseGroupStage {
		if m.GroupLabel == nil || *m.GroupLabel == "" {
			return apperrors.Validation("group_label is required for group_stage matches")
		}
		if _, ok := validGroupLabels[*m.GroupLabel]; !ok {
			return apperrors.Validation("group_label must be a single uppercase letter A–L")
		}
	} else if m.GroupLabel != nil {
		return apperrors.Validation("group_label must be absent for knockout matches")
	}
	return nil
}

// ValidateMatchResult checks that the supplied score pointers form a valid
// final result before they are persisted on an existing Match.
func ValidateMatchResult(homeScore, awayScore *int) error {
	if homeScore == nil || awayScore == nil {
		return apperrors.Validation("home score and away score must both be provided")
	}
	if *homeScore < 0 {
		return apperrors.Validation("home score must not be negative")
	}
	if *awayScore < 0 {
		return apperrors.Validation("away score must not be negative")
	}
	return nil
}

// ValidatePrediction checks that a Prediction carries a plausible scoreline
// and that it was submitted before the match deadline.
//
// deadlineOffset is subtracted from kickoffAt to derive the closing time;
// pass PredictionDeadlineOffset as the default or read a runtime value from
// SystemParamService. Accepting now and deadlineOffset as parameters makes the
// function fully deterministic: tests can inject any reference time and offset
// without racing against the real clock.
func ValidatePrediction(p *Prediction, kickoffAt, now time.Time, deadlineOffset time.Duration) error {
	if p.HomeScore < 0 {
		return apperrors.Validation("predicted home score must not be negative")
	}
	if p.AwayScore < 0 {
		return apperrors.Validation("predicted away score must not be negative")
	}
	deadline := kickoffAt.Add(-deadlineOffset)
	if now.After(deadline) {
		return apperrors.Validation("predictions are no longer accepted for this match")
	}
	return nil
}

// ValidateQuiniela checks that the essential fields of a Quiniela are present
// and within length bounds.
//
// entry_fee must be non-negative: zero means the group is free; positive values
// trigger the payment workflow. Negative values are rejected here so that the
// database CHECK constraint is never the first line of defence.
func ValidateQuiniela(q *Quiniela) error {
	if q.Name == "" {
		return apperrors.Validation("quiniela name must not be empty")
	}
	if len(q.Name) > MaxNameLength {
		return apperrors.Validation("quiniela name must not exceed 200 characters")
	}
	if q.OwnerID == 0 {
		return apperrors.Validation("quiniela must have an owner")
	}
	if q.EntryFee < 0 {
		return apperrors.Validation("entry fee must not be negative")
	}
	return nil
}

// ValidateGroupSize returns a validation error when n falls outside the
// platform-enforced membership bounds [MinMembersPerGroup, MaxMembersPerGroup].
//
// This function encapsulates the group-size business rule so that any future
// change to the bounds requires only one edit here, rather than scattered
// comparisons across the service layer. It is intended for use in service
// methods that receive an explicit member count (e.g. admin bulk operations).
// For join/leave operations the authoritative check is enforceMaxMembers inside
// RequestJoinByInviteCode and ApproveMembership, both of which read the runtime
// limit from system_params via ParamKeyGroupMaxSize.
func ValidateGroupSize(n int) error {
	if n < MinMembersPerGroup {
		return apperrors.Validation(
			"group must have at least 5 active members to be eligible for payments and prizes",
		)
	}
	if n > MaxMembersPerGroup {
		return apperrors.Validation(
			"group cannot have more than 20 active members",
		)
	}
	return nil
}

// supportedWithdrawalCurrencies is the canonical set of ISO-4217 currency
// codes accepted for withdrawal requests.
//
// GTQ (Guatemalan Quetzal) is the primary currency for domestic bank transfers.
// USD (US Dollar) is accepted for international PayPal payouts.
// Any other string is rejected at the handler boundary so it never reaches the
// DB or the ledger, keeping financial records internally consistent.
var supportedWithdrawalCurrencies = map[string]struct{}{
	"GTQ": {},
	"USD": {},
}

// ValidateWithdrawalCurrency returns a validation error when currency is not
// one of the platform-supported codes.  Pass the already-defaulted value (i.e.
// call this after the "empty → GTQ" fallback in the handler).
func ValidateWithdrawalCurrency(currency string) error {
	if _, ok := supportedWithdrawalCurrencies[currency]; !ok {
		return apperrors.Validation(`currency must be one of: "GTQ", "USD"`)
	}
	return nil
}

// validWinMethods is the canonical set of accepted WinMethod string values.
var validWinMethods = map[WinMethod]struct{}{
	WinMethodNormal:    {},
	WinMethodExtraTime: {},
	WinMethodPenalties: {},
}

// ParseWinMethod validates s against the three known WinMethod constants and
// returns the typed value. Returns a validation error for any unrecognised
// string so callers receive a 422 rather than a raw PostgreSQL CHECK violation.
func ParseWinMethod(s string) (WinMethod, error) {
	wm := WinMethod(s)
	if _, ok := validWinMethods[wm]; !ok {
		return "", apperrors.Validation(`win_method must be one of: "normal", "extra_time", "penalties"`)
	}
	return wm, nil
}

// validPhases is the set of recognised MatchPhase values. It is used by
// ValidateMatchPhase to reject arbitrary strings before they reach the DB.
var validPhases = map[MatchPhase]struct{}{
	PhaseGroupStage:   {},
	PhaseRoundOf32:    {},
	PhaseRoundOf16:    {},
	PhaseQuarterFinal: {},
	PhaseSemiFinal:    {},
	PhaseThirdPlace:   {},
	PhaseFinal:        {},
}

// payoutRequiredKeys maps each WithdrawalMethod to its required payout_details
// keys and the maximum character length for each value.  An exhaustive key set
// is enforced (no extra keys allowed) to prevent storage abuse and to ensure
// the admin payout workflow always has the fields it expects.
var payoutRequiredKeys = map[WithdrawalMethod]map[string]int{
	WithdrawalMethodBankGT: {
		"account_number": 30, // Guatemalan account numbers are ≤ 13 digits; 30 gives headroom
		"bank_name":      100,
	},
	WithdrawalMethodPayPal: {
		"paypal_email": MaxEmailLength, // reuse RFC 5321 maximum (320)
	},
}

// ValidatePayoutDetails checks that details contains exactly the required keys
// for method, that no unknown keys are present, and that all values satisfy
// their length bounds.  For PayPal the email format is also validated.
//
// Call this in the HTTP handler layer (before the service) so that invalid
// requests are rejected with 422 before any DB interaction.
func ValidatePayoutDetails(method WithdrawalMethod, details map[string]string) error {
	keyLimits, ok := payoutRequiredKeys[method]
	if !ok {
		return apperrors.Validation(fmt.Sprintf("unknown withdrawal method: %q", method))
	}
	if len(details) == 0 {
		return apperrors.Validation("payout_details must not be empty")
	}
	// Reject unknown keys first so callers get a precise error.
	for k := range details {
		if _, allowed := keyLimits[k]; !allowed {
			return apperrors.Validation(fmt.Sprintf("payout_details: unexpected key %q for method %q", k, method))
		}
	}
	// Verify all required keys are present and within length bounds.
	for k, maxLen := range keyLimits {
		v, present := details[k]
		if !present || v == "" {
			return apperrors.Validation(fmt.Sprintf("payout_details: %q is required for method %q", k, method))
		}
		if len(v) > maxLen {
			return apperrors.Validation(fmt.Sprintf("payout_details: %q must not exceed %d characters", k, maxLen))
		}
	}
	if method == WithdrawalMethodPayPal {
		if err := ValidateEmail(details["paypal_email"]); err != nil {
			return apperrors.Validation("payout_details: paypal_email is not a valid email address")
		}
	}
	return nil
}

// ValidateMatchPhase returns a validation error when phase is not one of the
// recognised FIFA World Cup 2026 tournament phases. An empty string is treated
// as "no phase filter" and is therefore valid (returns nil).
func ValidateMatchPhase(phase MatchPhase) error {
	if phase == "" {
		return nil
	}
	if _, ok := validPhases[phase]; !ok {
		return apperrors.Validation("phase is not a recognised tournament phase")
	}
	return nil
}

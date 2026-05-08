package domain_test

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

func TestWinnerCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want int
	}{
		// Below minimum — no prizes
		{name: "zero members", n: 0, want: 0},
		{name: "one member", n: 1, want: 0},
		{name: "four members — below minimum", n: 4, want: 0},

		// Tier 1: exactly 5 → Top 2
		{name: "five members — tier 1 lower bound", n: 5, want: 2},

		// Tier 2: 6–9 → Top 3
		{name: "six members — tier 2 lower bound", n: 6, want: 3},
		{name: "eight members — tier 2 mid", n: 8, want: 3},
		{name: "nine members — tier 2 upper bound", n: 9, want: 3},

		// Tier 3: 10–14 → Top 4
		{name: "ten members — tier 3 lower bound", n: 10, want: 4},
		{name: "twelve members — tier 3 mid", n: 12, want: 4},
		{name: "fourteen members — tier 3 upper bound", n: 14, want: 4},

		// Tier 4: 15–20 → Top 5
		{name: "fifteen members — tier 4 lower bound", n: 15, want: 5},
		{name: "eighteen members — tier 4 mid", n: 18, want: 5},
		{name: "twenty members — platform max", n: 20, want: 5},

		// Above platform cap — capped at 5 (defensive)
		{name: "twenty-one members — above platform cap", n: 21, want: 5},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.WinnerCount(tc.n)
			if got != tc.want {
				t.Errorf("WinnerCount(%d) = %d, want %d", tc.n, got, tc.want)
			}
		})
	}
}

func TestEligibleForPayments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want bool
	}{
		// Ineligible — below minimum
		{name: "zero members", n: 0, want: false},
		{name: "one member", n: 1, want: false},
		{name: "four members — one below threshold", n: 4, want: false},

		// Eligible — at or above minimum
		{name: "five members — exactly at threshold", n: 5, want: true},
		{name: "six members", n: 6, want: true},
		{name: "ten members", n: 10, want: true},
		{name: "twenty members — platform max", n: 20, want: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.EligibleForPayments(tc.n)
			if got != tc.want {
				t.Errorf("EligibleForPayments(%d) = %v, want %v", tc.n, got, tc.want)
			}
		})
	}
}

// TestWinnerCountAndEligibilityConsistency verifies that WinnerCount and
// EligibleForPayments agree on the boundary: any n that returns WinnerCount > 0
// must also return EligibleForPayments = true, and vice versa.
func TestWinnerCountAndEligibilityConsistency(t *testing.T) {
	t.Parallel()

	for n := 0; n <= 25; n++ {
		eligible := domain.EligibleForPayments(n)
		winners := domain.WinnerCount(n)

		if eligible && winners == 0 {
			t.Errorf("n=%d: EligibleForPayments=true but WinnerCount=0 — inconsistent", n)
		}
		if !eligible && winners > 0 {
			t.Errorf("n=%d: EligibleForPayments=false but WinnerCount=%d — inconsistent", n, winners)
		}
	}
}

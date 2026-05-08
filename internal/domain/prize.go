package domain

// WinnerCount returns the number of prize positions for a group with n active
// paid members. The tiers are fixed platform rules:
//
//	n < 5        → 0  (group below minimum; no prizes distributed)
//	n = 5        → 2  (Top 2)
//	6  ≤ n ≤ 9  → 3  (Top 3)
//	10 ≤ n ≤ 14 → 4  (Top 4)
//	15 ≤ n ≤ 20 → 5  (Top 5)
//
// Groups larger than MaxMembersPerGroup (20) cannot be formed by platform
// rules, so values above 20 return 5 as a safe upper bound rather than
// panicking. The caller is responsible for ensuring n reflects only active,
// paid members — pending or left memberships must not be counted.
func WinnerCount(n int) int {
	switch {
	case n < MinMembersPerGroup:
		return 0
	case n == MinMembersPerGroup: // exactly 5
		return 2
	case n <= 9:
		return 3
	case n <= 14:
		return 4
	default: // 15–20 (and any value above the platform cap)
		return 5
	}
}

// EligibleForPayments reports whether a group with n active paid members has
// reached the minimum threshold required for payment processing and prize
// distribution. Groups below MinMembersPerGroup (5) remain in
// QuinielaStatusInactive and must not have payments validated or prizes
// assigned, regardless of their entry_fee setting.
//
// This is the single authoritative check for payment/prize eligibility.
// All service-layer code that gates payment or prize logic must call this
// function rather than comparing against MinMembersPerGroup directly, so
// that a future threshold change requires only one edit.
func EligibleForPayments(n int) bool {
	return n >= MinMembersPerGroup
}

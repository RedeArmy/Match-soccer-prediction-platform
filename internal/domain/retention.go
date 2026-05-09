package domain

// RetentionTier classifies how a table's rows are handled during a GDPR
// erasure request (EraseUserPII) and after the soft-delete retention window.
//
// This platform handles real money (entry fees and prize pools), so data
// retention is a legal requirement governed by GAAP and SAT record-keeping
// rules, not an optional best-effort policy.
//
// # Table classification
//
//	Table                   Tier                   Notes
//	───────────────────────────────────────────────────────────────────────
//	users                   SoftDelete             30-day window then hard-delete
//	quinielas               SoftDelete             30-day window then hard-delete
//	group_memberships       OperationalDelete      Cascade via FK on user hard-delete
//	predictions             OperationalDelete      Deleted by EraseUserPII
//	tiebreakers             OperationalDelete      Deleted by EraseUserPII
//	payment_records         ImmutableAnonymise     GAAP/SAT — row kept, user_id → NULL
//	audit_log               ImmutableAnonymise     Compliance — row kept, actor_id → NULL
//	leaderboard_snapshots   Reference              Aggregate; not directly user-owned
//	matches                 Reference              Fixture data; admin-managed
//	stadiums / cities / …   Reference              Geographic reference data
//	system_params           Reference              Configuration; not user-owned
//	tournament_slots        Reference              Bracket data; admin-managed
//	tiebreaker_config       Reference              Question config; admin-managed
type RetentionTier int

const (
	// RetentionSoftDelete rows are soft-deleted when a user account is closed
	// and are hard-deleted after the configured retention window (default 30
	// days). EraseUserPII does not touch these rows directly; the hard-delete
	// purge handles them after the window expires.
	RetentionSoftDelete RetentionTier = iota

	// RetentionOperationalDelete rows carry no financial or legal obligation and
	// are deleted immediately by EraseUserPII as part of the erasure request.
	RetentionOperationalDelete

	// RetentionImmutableAnonymise rows must be preserved for compliance (GAAP /
	// SAT financial records; compliance audit trail) but must not identify the
	// data subject after erasure. EraseUserPII sets the user FK to NULL; the
	// row itself is never deleted.
	RetentionImmutableAnonymise

	// RetentionReference rows are shared reference data that are never
	// user-owned and are never modified by an erasure request.
	RetentionReference
)

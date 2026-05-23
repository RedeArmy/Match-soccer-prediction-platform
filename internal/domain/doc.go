// Package domain contains the core business entities and rules of the
// World Cup quiniela system.
//
// This package must remain entirely free of infrastructure concerns: no
// database drivers, no HTTP types, no serialisation tags, no external
// library dependencies. The entities here represent concepts that the
// business cares about - Users, Matches, Predictions - not how they are
// stored in PostgreSQL or transported over HTTP.
//
// This boundary is what makes the business logic testable in isolation
// and portable across different storage or transport technologies. If you
// find yourself importing a third-party package here, stop and reconsider
// the design: that dependency almost certainly belongs in the infrastructure
// or service layer instead.
//
// File layout (one aggregate per file):
//
//	entity_user.go         — User, UserRole
//	entity_location.go     — Country, State, City, Stadium
//	entity_match.go        — MatchPhase, MatchStatus, WinMethod, Match,
//	                         ScoringRule, GroupStanding, TournamentSlot
//	entity_quiniela.go     — Quiniela, GroupMembership, MembershipRole,
//	                         LeaderboardEntry, Tiebreaker, Snapshot types
//	entity_system.go       — SystemParam, SystemParamHistory
//	entity_admin.go        — AuditLog, Conflict, DashboardStats
//	entity_payment.go      — PaymentRecord, BalanceLedger, BankTransferProof,
//	                         WithdrawalRequest
//	entity_notification.go — AdminNotificationLog, UserNotification,
//	                         NotificationTemplate, PushSubscription
package domain

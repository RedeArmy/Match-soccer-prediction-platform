package domain

import "time"

// User represents a registered participant in the quiniela platform.
//
// Authentication is delegated to an external identity provider via the
// auth.IdentityProvider interface. No password or credential is stored here.
// ExternalSubject is the opaque "sub" claim the provider stamps on each JWT
// (e.g. "user_2abc…" for Clerk, "auth0|…" for Auth0) and is the stable link
// between the external identity and the internal User record.
//
// BannedAt/BannedBy/BanReason track administrative bans. A non-nil BannedAt
// means the user is currently banned and must be blocked from all write
// operations. BannedBy is the ID of the admin who issued the ban; BanReason
// is a human-readable explanation stored for audit purposes.
type User struct {
	ID              int
	Name            string
	Username        string // Clerk username; empty when the user has not set one
	Email           string
	Role            UserRole
	ExternalSubject string // opaque identity-provider subject (JWT "sub"); empty for legacy rows
	BannedAt        *time.Time
	BannedBy        *int
	BanReason       string
	BalanceCents    int     // spendable funds in minor currency units; never negative
	ReservedCents   int     // funds locked for pending withdrawal requests
	KYCTier         KYCTier // denormalised from kyc_profiles.tier; updated by KYCService
	Locale          string  // BCP-47 tag ("es" or "en"); defaults to "es" for new users
	Timezone        string  // IANA timezone (e.g. "America/Guatemala"); defaults to "America/Guatemala"
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time // nil for active users; set when the record is soft-deleted
}

// UserRole is a typed string that constrains the roles a User may hold.
//
// Using a named type rather than a bare string prevents accidental comparisons
// against untyped string literals and makes exhaustive switch statements
// possible when combined with a linter that enforces exhaustiveness checks.
// New roles must be added to this block explicitly; they cannot be introduced
// silently by passing an arbitrary string.
type UserRole string

// Allowed values for UserRole.
const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

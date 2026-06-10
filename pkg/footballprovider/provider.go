// Package footballprovider defines the interface and types for external
// football data providers. The concrete implementation targets API-Football v3
// (api-sports.io); the interface makes alternative providers injectable
// in tests without an HTTP server dependency.
package footballprovider

import (
	"context"
	"time"
)

// FixtureStatus mirrors the short status codes returned by API-Football.
// The full list has 16 values; only those relevant to the sync worker are
// enumerated here. Unknown codes are mapped to StatusUnknown so the worker
// can log and skip them gracefully.
type FixtureStatus string

const (
	StatusNotStarted FixtureStatus = "NS"        // Not Started
	StatusFirstHalf  FixtureStatus = "1H"        // First Half
	StatusHalfTime   FixtureStatus = "HT"        // Half Time
	StatusSecondHalf FixtureStatus = "2H"        // Second Half
	StatusExtraTime  FixtureStatus = "ET"        // Extra Time
	StatusPenLive    FixtureStatus = "PEN_LIVE"  // Penalties in progress
	StatusFullTime   FixtureStatus = "FT"        // Full Time
	StatusAfterET    FixtureStatus = "AET"       // After Extra Time
	StatusAfterPEN   FixtureStatus = "PEN"       // After Penalties
	StatusPostponed  FixtureStatus = "PST"       // Postponed
	StatusCancelled  FixtureStatus = "CANC"      // Cancelled
	StatusAbandoned  FixtureStatus = "ABD"       // Abandoned
	StatusUnknown    FixtureStatus = "UNKNOWN"   // unrecognised / not yet supported
)

// IsLive reports whether the fixture is currently in play (including extra
// time and live penalty shootout). The sync worker uses this to decide
// between the fast and slow polling intervals.
func (s FixtureStatus) IsLive() bool {
	switch s {
	case StatusFirstHalf, StatusHalfTime, StatusSecondHalf, StatusExtraTime, StatusPenLive:
		return true
	default:
		return false
	}
}

// IsFinished reports whether the fixture has a confirmed final result.
func (s FixtureStatus) IsFinished() bool {
	switch s {
	case StatusFullTime, StatusAfterET, StatusAfterPEN:
		return true
	default:
		return false
	}
}

// IsCancelled reports whether the fixture will not produce a scorable result.
func (s FixtureStatus) IsCancelled() bool {
	switch s {
	case StatusPostponed, StatusCancelled, StatusAbandoned:
		return true
	default:
		return false
	}
}

// Fixture is the provider-agnostic representation of a single match returned
// by the external API. Only the fields consumed by the sync worker are present;
// additional raw data is discarded at the decode layer.
type Fixture struct {
	ExternalID int64
	Status     FixtureStatus
	HomeScore  int
	AwayScore  int
	KickoffUTC time.Time
}

// Client is the interface every football data provider must satisfy.
// It is kept narrow intentionally: the sync worker only needs two operations.
type Client interface {
	// GetFixture fetches the current state of a single fixture by its
	// provider-assigned ID. Returns an error when the request fails or
	// the fixture is not found.
	GetFixture(ctx context.Context, externalID int64) (*Fixture, error)

	// GetLiveFixtures returns all fixtures currently in play for the given
	// league and season. Used by the slow-to-fast interval upgrade logic.
	GetLiveFixtures(ctx context.Context, leagueID, season int) ([]*Fixture, error)
}

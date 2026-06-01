package service

// White-box tests for exchangeRateCache — package service gives access to
// the unexported type without needing to route through OTel instrumentation.
//
// ageSeconds() drives the wcq_fx_rate_age_seconds observable gauge that fires
// the WCQExchangeRateStale alert when the last successful fetch is too old.
// Its two branches (zero when never fetched; elapsed seconds after a fetch)
// must both be correct: a wrong zero branch would suppress stale alerts
// permanently, and a wrong elapsed branch would fire them at the wrong time.

import (
	"testing"
	"time"
)

func TestExchangeRateCache_AgeSeconds_ZeroWhenNeverFetched(t *testing.T) {
	// A freshly created cache has no lastFetchedAt set. ageSeconds must return
	// exactly 0 so the WCQExchangeRateStale alert does not fire before the
	// first scheduled refresh has had a chance to run.
	c := newExchangeRateCache(time.Hour)
	if age := c.ageSeconds(); age != 0 {
		t.Errorf("ageSeconds on uninitialised cache: got %v, want 0", age)
	}
}

func TestExchangeRateCache_AgeSeconds_ReflectsTimeSinceLastFetch(t *testing.T) {
	// After a fetch the age must reflect the elapsed time since fetchedAt, NOT
	// since cachedAt (time.Now() at set time). These differ when RefreshRate
	// stores a rate with a historical effective_at from the provider response.
	// The stale alert threshold is measured against the provider fetch time, so
	// both time origins must be distinct and ageSeconds must use the right one.
	c := newExchangeRateCache(time.Hour)

	const fetchedSecondsAgo = 10
	fetchedAt := time.Now().Add(-fetchedSecondsAgo * time.Second)
	c.set(nil, fetchedAt) // nil rates is fine; ageSeconds only reads lastFetchedAt

	age := c.ageSeconds()
	// Allow generous bounds to avoid CI flakiness on slow runners.
	const wantMin = fetchedSecondsAgo - 1 // at least 9s
	const wantMax = fetchedSecondsAgo + 5 // at most 15s
	if age < wantMin {
		t.Errorf("ageSeconds: got %.2fs, want >= %.0fs (elapsed since fetch)", age, float64(wantMin))
	}
	if age > wantMax {
		t.Errorf("ageSeconds: got %.2fs, want <= %.0fs (possible test clock skew)", age, float64(wantMax))
	}
}

func TestExchangeRateCache_AgeSeconds_ZeroWhenFetchedAtIsExplicitlyZero(t *testing.T) {
	// set() is a no-op on lastFetchedAt when fetchedAt.IsZero() (e.g. admin
	// overrides that carry no external fetch timestamp). A subsequent ageSeconds()
	// call must still return 0, not a large negative or stale value.
	c := newExchangeRateCache(time.Hour)
	c.set(nil, time.Time{}) // zero fetchedAt → lastFetchedAt unchanged
	if age := c.ageSeconds(); age != 0 {
		t.Errorf("ageSeconds after set with zero fetchedAt: got %v, want 0", age)
	}
}

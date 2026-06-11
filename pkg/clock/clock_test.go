package clock_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/clock"
)

func TestReal_NowIsUTC(t *testing.T) {
	before := time.Now().UTC()
	got := clock.Real{}.Now()
	after := time.Now().UTC()

	if got.Location() != time.UTC {
		t.Errorf("Real.Now() location = %v, want UTC", got.Location())
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("Real.Now() = %v, want between %v and %v", got, before, after)
	}
}

func TestFrozen_NowReturnsPinnedTime(t *testing.T) {
	pinned := time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC)
	got := clock.Frozen{T: pinned}.Now()

	if !got.Equal(pinned) {
		t.Errorf("Frozen.Now() = %v, want %v", got, pinned)
	}
}

func TestFrozen_NowIsStable(t *testing.T) {
	pinned := time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC)
	c := clock.Frozen{T: pinned}

	for i := range 5 {
		got := c.Now()
		if !got.Equal(pinned) {
			t.Errorf("call %d: Frozen.Now() = %v, want %v", i, got, pinned)
		}
	}
}

func TestClockInterface_Satisfied(t *testing.T) {
	var _ clock.Nower = clock.Real{}
	var _ clock.Nower = clock.Frozen{}
	var _ clock.Nower = clock.NewParamClock(stubGetter(""), "system.date", false)
}

// stubGetter implements clock.ParamGetter for tests, always returning its string value.
type stubGetter string

func (s stubGetter) GetString(_ context.Context, _, _ string) string { return string(s) }

// TestParamClock_ProdMode verifies that the override is ignored when isDev=false.
func TestParamClock_ProdMode(t *testing.T) {
	const override = "2026-07-01T20:00:00Z"
	clk := clock.NewParamClock(stubGetter(override), "system.date", false)

	before := time.Now().UTC()
	got := clk.Now()
	after := time.Now().UTC()

	if got.Before(before) || got.After(after) {
		t.Errorf("production ParamClock.Now() = %v, want real time between %v and %v", got, before, after)
	}
}

// TestParamClock_DevMode_EmptyParam falls back to real time when param is empty.
func TestParamClock_DevMode_EmptyParam(t *testing.T) {
	clk := clock.NewParamClock(stubGetter(""), "system.date", true)

	before := time.Now().UTC()
	got := clk.Now()
	after := time.Now().UTC()

	if got.Before(before) || got.After(after) {
		t.Errorf("dev ParamClock.Now() with empty param = %v, want real time between %v and %v", got, before, after)
	}
}

// TestParamClock_DevMode_ValidOverride returns the pinned time from the param.
func TestParamClock_DevMode_ValidOverride(t *testing.T) {
	const raw = "2026-07-01T20:00:00Z"
	want, _ := time.Parse(time.RFC3339, raw)
	clk := clock.NewParamClock(stubGetter(raw), "system.date", true)

	if got := clk.Now(); !got.Equal(want) {
		t.Errorf("dev ParamClock.Now() = %v, want %v", got, want)
	}
}

// TestParamClock_DevMode_InvalidParam falls back to real time when value is not RFC3339.
func TestParamClock_DevMode_InvalidParam(t *testing.T) {
	clk := clock.NewParamClock(stubGetter("not-a-date"), "system.date", true)

	before := time.Now().UTC()
	got := clk.Now()
	after := time.Now().UTC()

	if got.Before(before) || got.After(after) {
		t.Errorf("dev ParamClock.Now() with invalid param = %v, want real time between %v and %v", got, before, after)
	}
}

// TestParamClock_DevMode_UTC verifies the returned time is always UTC.
func TestParamClock_DevMode_UTC(t *testing.T) {
	const raw = "2026-07-01T14:00:00-06:00"
	clk := clock.NewParamClock(stubGetter(raw), "system.date", true)
	got := clk.Now()

	if got.Location() != time.UTC {
		t.Errorf("ParamClock.Now() location = %v, want UTC", got.Location())
	}
	if got.Hour() != 20 {
		t.Errorf("ParamClock.Now() hour after UTC conversion = %d, want 20", got.Hour())
	}
}

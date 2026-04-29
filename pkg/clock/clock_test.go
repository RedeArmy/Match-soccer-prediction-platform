package clock_test

import (
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
	var _ clock.Clock = clock.Real{}
	var _ clock.Clock = clock.Frozen{}
}

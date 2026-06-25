package breaker_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

var errFail = errors.New("failure")

func alwaysFail() error    { return errFail }
func alwaysSucceed() error { return nil }

// newFast returns a breaker that opens after 2 failures and stays open for
// 50 ms — short enough for tests to advance past the cooldown.
func newFast(t *testing.T) *breaker.Breaker {
	t.Helper()
	return breaker.New(t.Name(), 2, 50*time.Millisecond)
}

// ── State transitions ─────────────────────────────────────────────────────────

func TestBreaker_Initial_StateClosed(t *testing.T) {
	b := newFast(t)
	if got := b.CurrentState(); got != breaker.StateClosed {
		t.Errorf("expected Closed, got %s", got)
	}
}

func TestBreaker_UnderThreshold_RemainsClosedOnError(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail) // 1 failure — threshold is 2
	if got := b.CurrentState(); got != breaker.StateClosed {
		t.Errorf("expected Closed after 1 failure (threshold=2), got %s", got)
	}
}

func TestBreaker_AtThreshold_OpensOnError(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail) // 2nd failure → Open
	if got := b.CurrentState(); got != breaker.StateOpen {
		t.Errorf("expected Open after 2 failures, got %s", got)
	}
}

func TestBreaker_Open_ShortCircuitsWithErrOpen(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail)

	err := b.Call(alwaysFail)
	if !errors.Is(err, breaker.ErrOpen) {
		t.Errorf("expected ErrOpen when circuit is open, got %v", err)
	}
}

func TestBreaker_Open_DoesNotCallFn(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail)

	called := false
	_ = b.Call(func() error {
		called = true
		return nil
	})
	if called {
		t.Error("fn must not be called when circuit is open")
	}
}

func TestBreaker_AfterCooldown_TransitionsToHalfOpen(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail)

	time.Sleep(60 * time.Millisecond) // wait past openFor=50ms

	if got := b.CurrentState(); got != breaker.StateHalfOpen {
		t.Errorf("expected HalfOpen after cooldown, got %s", got)
	}
}

func TestBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail)
	time.Sleep(60 * time.Millisecond)

	_ = b.Call(alwaysSucceed) // trial succeeds → Closed
	if got := b.CurrentState(); got != breaker.StateClosed {
		t.Errorf("expected Closed after successful trial, got %s", got)
	}
}

func TestBreaker_HalfOpen_FailureReopens(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail)
	time.Sleep(60 * time.Millisecond)

	_ = b.Call(alwaysFail) // trial fails → Open again
	if got := b.CurrentState(); got != breaker.StateOpen {
		t.Errorf("expected Open after failed trial, got %s", got)
	}
}

func TestBreaker_SuccessResetsCounter(t *testing.T) {
	b := newFast(t)
	_ = b.Call(alwaysFail) // 1 failure
	_ = b.Call(alwaysSucceed)
	_ = b.Call(alwaysFail) // counter reset; 1 failure again
	if got := b.CurrentState(); got != breaker.StateClosed {
		t.Errorf("expected counter reset after success; got %s", got)
	}
}

func TestBreaker_SuccessPassesThroughResult(t *testing.T) {
	b := newFast(t)
	if err := b.Call(alwaysSucceed); err != nil {
		t.Errorf("expected nil from successful call, got %v", err)
	}
}

func TestBreaker_FailurePassesThroughOriginalError(t *testing.T) {
	b := newFast(t)
	err := b.Call(alwaysFail)
	if !errors.Is(err, errFail) {
		t.Errorf("expected errFail, got %v", err)
	}
}

// ── Name and State.String ─────────────────────────────────────────────────────

func TestBreaker_Name_ReturnsGiven(t *testing.T) {
	b := breaker.New("my-dep", 3, time.Second)
	if b.Name() != "my-dep" {
		t.Errorf("expected %q, got %q", "my-dep", b.Name())
	}
}

func TestState_String_AllValues(t *testing.T) {
	cases := []struct {
		state breaker.State
		want  string
	}{
		{breaker.StateClosed, "closed"},
		{breaker.StateOpen, "open"},
		{breaker.StateHalfOpen, "half-open"},
		{breaker.State(99), "unknown(99)"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// ── Constructor guards ────────────────────────────────────────────────────────

func TestNew_ZeroMaxFails_ClampedToOne(t *testing.T) {
	b := breaker.New("x", 0, time.Second)
	_ = b.Call(alwaysFail) // 1 failure must open it
	if got := b.CurrentState(); got != breaker.StateOpen {
		t.Errorf("expected Open after 1 failure with clamped maxFails=1, got %s", got)
	}
}

func TestNew_NegativeOpenFor_ClampedToPositive(t *testing.T) {
	b := breaker.New("x", 1, -time.Second)
	_ = b.Call(alwaysFail) // open it
	// After instant cooldown (clamped to 1s), state is still open immediately
	if got := b.CurrentState(); got != breaker.StateOpen {
		t.Errorf("expected Open immediately after opening, got %s", got)
	}
}

// ── OpenedAt ─────────────────────────────────────────────────────────────────

func TestBreaker_OpenedAt_ZeroBeforeAnyFailure(t *testing.T) {
	b := newFast(t)
	if !b.OpenedAt().IsZero() {
		t.Error("expected zero OpenedAt before any call")
	}
}

func TestBreaker_OpenedAt_NonZeroAfterOpen(t *testing.T) {
	before := time.Now()
	b := newFast(t)
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail) // opens
	if ts := b.OpenedAt(); ts.Before(before) || ts.IsZero() {
		t.Errorf("expected OpenedAt >= %v, got %v", before, ts)
	}
}

// ── SetOnStateChange ─────────────────────────────────────────────────────────

func TestBreaker_SetOnStateChange_FiredOnTransition(t *testing.T) {
	b := newFast(t)
	var events []string
	b.SetOnStateChange(func(name string, from, to breaker.State, _ time.Time) {
		events = append(events, from.String()+"->"+to.String())
	})
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail) // closed → open

	if len(events) == 0 {
		t.Fatal("expected at least one state-change event")
	}
	if events[len(events)-1] != "closed->open" {
		t.Errorf("last event: got %q, want %q", events[len(events)-1], "closed->open")
	}
}

func TestBreaker_SetOnStateChange_NilClearsHook(t *testing.T) {
	b := newFast(t)
	fired := false
	b.SetOnStateChange(func(string, breaker.State, breaker.State, time.Time) { fired = true })
	b.SetOnStateChange(nil) // clear
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail) // opens — hook must not fire
	if fired {
		t.Error("hook fired after being cleared with nil")
	}
}

func TestBreaker_SetOnStateChange_PanicInHookDoesNotCrash(t *testing.T) {
	b := newFast(t)
	b.SetOnStateChange(func(string, breaker.State, breaker.State, time.Time) {
		panic("deliberate panic in hook")
	})
	// Must not panic — safeCallHook recovers it.
	_ = b.Call(alwaysFail)
	_ = b.Call(alwaysFail)
}

// ── Registry ──────────────────────────────────────────────────────────────────

func TestRegistry_NewRegistry_EmptyAll(t *testing.T) {
	reg := breaker.NewRegistry()
	if got := reg.All(); len(got) != 0 {
		t.Errorf("expected empty registry, got %d entries", len(got))
	}
}

func TestRegistry_Register_And_All(t *testing.T) {
	reg := breaker.NewRegistry()
	b1 := breaker.New("svc-a", 3, time.Second)
	b2 := breaker.New("svc-b", 3, time.Second)
	reg.Register(b1)
	reg.Register(b2)

	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 breakers, got %d", len(all))
	}
}

func TestRegistry_Register_Replaces_SameName(t *testing.T) {
	reg := breaker.NewRegistry()
	old := breaker.New("svc", 3, time.Second)
	newer := breaker.New("svc", 5, time.Second)
	reg.Register(old)
	reg.Register(newer)

	all := reg.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 breaker after replacement, got %d", len(all))
	}
	if all[0].Name() != "svc" {
		t.Errorf("unexpected name %q", all[0].Name())
	}
}

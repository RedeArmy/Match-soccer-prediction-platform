// Package breaker implements a simple, concurrency-safe circuit breaker.
//
// The breaker has three states:
//
//	Closed     — normal operation; all calls pass through.
//	Open       — all calls are short-circuited with ErrOpen for openFor duration
//	             after maxFails consecutive failures.
//	Half-Open  — one trial call is allowed after the open window expires;
//	             a success closes the circuit, a failure reopens it.
//
// Usage:
//
//	b := breaker.New("redis-cache", 5, 30*time.Second)
//	err := b.Call(func() error { return redisClient.Ping(ctx).Err() })
//	if errors.Is(err, breaker.ErrOpen) {
//	    // degrade gracefully
//	}
package breaker

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrOpen is returned when all calls are short-circuited because the breaker
// is in the Open state. Callers should degrade gracefully rather than
// propagating this error to end-users.
var ErrOpen = errors.New("circuit breaker open")

// State represents the current condition of the circuit breaker.
type State int

const (
	// StateClosed is the normal operating state: all calls pass through.
	StateClosed State = iota
	// StateOpen short-circuits all calls until the open window expires.
	StateOpen
	// StateHalfOpen allows a single trial call after the open window expires.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Breaker is a concurrency-safe circuit breaker. The zero value is not usable;
// construct with New.
type Breaker struct {
	name     string
	maxFails int
	openFor  time.Duration

	mu               sync.Mutex
	state            State
	consecutiveFails int
	openedAt         time.Time
}

// New returns a Breaker that opens after maxFails consecutive failures and
// stays open for openFor before transitioning to half-open.
//
// name is used only in log/metric strings; it does not affect behaviour.
// maxFails must be ≥ 1; openFor must be > 0.
func New(name string, maxFails int, openFor time.Duration) *Breaker {
	if maxFails < 1 {
		maxFails = 1
	}
	if openFor <= 0 {
		openFor = time.Second
	}
	return &Breaker{name: name, maxFails: maxFails, openFor: openFor}
}

// Name returns the breaker's name.
func (b *Breaker) Name() string { return b.name }

// CurrentState returns the current state of the breaker. It is safe for
// concurrent use but the state may change between this call and the next.
func (b *Breaker) CurrentState() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.resolveState()
}

// Call executes fn if the breaker allows it, records the outcome, and returns
// the result. When the circuit is open, fn is not called and ErrOpen is
// returned immediately so the caller can degrade without waiting for a timeout.
func (b *Breaker) Call(fn func() error) error {
	if err := b.allow(); err != nil {
		return err
	}
	err := fn()
	b.record(err)
	return err
}

// allow returns nil if the call should proceed, or ErrOpen if it must be
// short-circuited. It transitions the breaker to half-open when the open
// window has expired.
func (b *Breaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.resolveState()
	if state == StateOpen {
		return ErrOpen
	}
	return nil
}

// resolveState transitions from Open → HalfOpen if the cooldown has elapsed.
// Must be called with b.mu held.
func (b *Breaker) resolveState() State {
	if b.state == StateOpen && time.Since(b.openedAt) >= b.openFor {
		b.state = StateHalfOpen
	}
	return b.state
}

// record updates the failure counters and state machine based on whether fn
// returned an error. Must be called after every Call that was allowed.
func (b *Breaker) record(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.consecutiveFails = 0
		b.state = StateClosed
		return
	}
	b.consecutiveFails++
	b.openedAt = time.Now()
	if b.state == StateHalfOpen || b.consecutiveFails >= b.maxFails {
		b.state = StateOpen
	}
}

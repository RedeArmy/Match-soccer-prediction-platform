package clock

import "time"

// Clock abstracts wall-clock access for testability and determinism.
type Clock interface {
	Now() time.Time
}

// Real returns the current UTC wall-clock time.
type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

// Frozen always returns T. Inject in tests to drive deadline-sensitive logic
// without racing against the real clock.
type Frozen struct{ T time.Time }

func (f Frozen) Now() time.Time { return f.T }

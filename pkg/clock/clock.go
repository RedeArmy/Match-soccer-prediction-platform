package clock

import (
	"context"
	"time"
)

// Nower abstracts wall-clock access for testability and determinism.
type Nower interface {
	Now() time.Time
}

// Real returns the current UTC wall-clock time.
type Real struct{}

// Now returns the current wall-clock time in UTC.
func (Real) Now() time.Time { return time.Now().UTC() }

// Frozen always returns T. Inject in tests to drive deadline-sensitive logic
// without racing against the real clock.
type Frozen struct{ T time.Time }

// Now returns the pinned time T, unchanged on every call.
func (f Frozen) Now() time.Time { return f.T }

// ParamGetter is a narrow interface satisfied by service.SystemParamService.
// Defined here so pkg/clock does not import internal packages.
type ParamGetter interface {
	GetString(ctx context.Context, key, defaultVal string) string
}

// ParamClock reads the wall clock from a system parameter when running in
// development mode. Falls back to Real{} when:
//   - isDev is false (production mode), or
//   - the param value is empty or not a valid RFC 3339 timestamp.
//
// This lets operators pin the application clock via the admin panel during
// testing without affecting production behaviour.
type ParamClock struct {
	params ParamGetter
	key    string
	isDev  bool
}

// NewParamClock constructs a ParamClock. key is the system-param key whose
// value will be consulted on every Now() call. isDev must be cfg.IsDevelopment().
func NewParamClock(params ParamGetter, key string, isDev bool) ParamClock {
	return ParamClock{params: params, key: key, isDev: isDev}
}

// Now returns the overridden time when in dev mode and the param holds a valid
// RFC 3339 value; otherwise it returns the real UTC wall-clock time.
func (c ParamClock) Now() time.Time {
	if c.isDev {
		raw := c.params.GetString(context.Background(), c.key, "")
		if raw != "" {
			if t, err := time.Parse(time.RFC3339, raw); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Now().UTC()
}

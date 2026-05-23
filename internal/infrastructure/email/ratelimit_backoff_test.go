package email

import (
	"testing"
	"time"
)

func TestRateLimitBackoff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // cap
		{10, 30 * time.Second},
		{100, 30 * time.Second},
	}

	for _, tc := range cases {
		got := rateLimitBackoff(tc.attempt)
		if got != tc.want {
			t.Errorf("rateLimitBackoff(%d) = %v; want %v", tc.attempt, got, tc.want)
		}
	}
}

package repository

import "time"

// RetryPolicySnapshot returns the current retry policy values for assertion in
// tests. Only valid for use in tests; not part of the public API.
func RetryPolicySnapshot() (maxAttempts int, baseDelay, maxDelay time.Duration) {
	return retryMaxAttempts, retryBaseDelay, retryMaxDelay
}

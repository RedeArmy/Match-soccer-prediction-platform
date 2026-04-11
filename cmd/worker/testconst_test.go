package main

// testconst_test.go centralises string literals used across multiple test
// files in this package to satisfy the "no duplicate literals" lint rule.
const (
	fmtUnexpectedErr = "unexpected error: %v"
	driverRedis      = "redis"
	teamMexico       = "Mexico"
	pathHealth       = "/health"
	pathHealthReady  = "/health/ready"
)

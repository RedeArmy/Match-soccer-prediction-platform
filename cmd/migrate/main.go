// Command migrate applies pending database schema migrations.
//
// This binary is intentionally separate from the API server so that
// migrations can be executed as a Kubernetes init container or a one-off
// job, guaranteeing the schema is up to date before any API pod begins
// serving traffic. Running migrations inside the API binary itself creates
// a race condition when multiple replicas start simultaneously — each
// instance would attempt to apply the same migration, requiring distributed
// locking to prevent corruption.
package main

import (
	"fmt"
	"os"
)

func main() {
	// TODO: implement schema migration runner (e.g. golang-migrate/migrate).
	fmt.Fprintln(os.Stderr, "migrate: not yet implemented")
	os.Exit(1)
}

// Command worker runs background processing tasks such as score calculation
// and notification dispatch.
//
// Long-running or CPU-intensive operations are deliberately offloaded to this
// separate binary rather than executed inside the API server. This prevents
// background work from competing with HTTP handlers for goroutines, file
// descriptors, and database connections, and allows the worker to be scaled
// independently based on queue depth rather than request rate.
package main

import (
	"fmt"
	"os"
)

func main() {
	// TODO: implement event consumer and task dispatcher.
	fmt.Fprintln(os.Stderr, "worker: not yet implemented")
	os.Exit(1)
}

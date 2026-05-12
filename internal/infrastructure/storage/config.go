package storage

// Config holds the configuration required to construct a FileStore.
type Config struct {
	// Driver selects the backing store implementation.
	// Accepted values: "local".
	Driver string

	// LocalDir is the filesystem root used by the "local" driver.
	// Defaults to "uploads" relative to the working directory.
	LocalDir string
}

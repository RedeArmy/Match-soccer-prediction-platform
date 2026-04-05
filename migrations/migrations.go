// Package migrations embeds the SQL migration files into the binary at
// compile time using go:embed.
//
// Embedding the files avoids any dependency on the filesystem path at runtime:
// the binary is self-contained and can be deployed to any environment without
// copying the migrations/ directory alongside it.
//
// Both cmd/api (auto-migration on startup) and cmd/migrate (manual runner)
// import this package to obtain the same fs.FS, ensuring a single source of
// truth for the SQL files regardless of which binary is executing them.
package migrations

import "embed"

// FS contains all *.sql files in the migrations directory.
// The embed directive must be in the same package as the files it embeds.
//
//go:embed *.sql
var FS embed.FS

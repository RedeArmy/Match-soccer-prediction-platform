package api

import "embed"

// staticFiles embeds the contents of the static/ directory into the binary.
// Files are served at root paths (/sw.js, /push.js, /icons/*) by Routes().
//
//go:embed static
var staticFiles embed.FS

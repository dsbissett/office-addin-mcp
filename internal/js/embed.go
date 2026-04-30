// Package js owns the embedded Office.js payload sources. They live here
// rather than under internal/officejs because //go:embed cannot reach into a
// parent directory.
package js

import "embed"

// FS holds every .js file in this directory. _preamble.js is concatenated
// before every excel_* payload by the officejs executor. The `all:` prefix
// is required because Go's embed otherwise skips files whose names start
// with '_' or '.'.
//
//go:embed all:*.js
var FS embed.FS

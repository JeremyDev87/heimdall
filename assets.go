package heimdallassets

import "embed"

// FS contains the public JSON schemas used by the evaluator.
//
//go:embed schemas/*.json
var FS embed.FS

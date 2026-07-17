package migrations

import "embed"

// FS holds the goose SQL migrations, embedded into the binary so the API
// self-migrates on boot (no separate migrate container).
//
//go:embed *.sql
var FS embed.FS

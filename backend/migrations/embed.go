package migrations

import "embed"

// FS contains SQLite migration files for single-binary deployment.
//
//go:embed *.sql
var FS embed.FS

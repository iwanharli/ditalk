// Package migrations embeds the SQL migration files so the compiled binary can
// bootstrap its own schema without the goose CLI installed.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

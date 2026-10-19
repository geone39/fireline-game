// Package migrations embeds goose SQL migrations.
//
// Rule: migrations are forward-compatible (expand/contract): a release only adds;
// drops and renames ship one release after the code stopped using the old shape.
package migrations

import "embed"

// FS holds all *.sql migration files.
//
//go:embed *.sql
var FS embed.FS

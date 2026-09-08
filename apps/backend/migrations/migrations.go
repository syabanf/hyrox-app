// Package migrations embeds the SQL schema so a built binary carries its own
// migrations: deploying the image is enough, with no separate SQL bundle to
// keep in step with it.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// Package static embeds the repo's top-level static/ directory (CSS, JS)
// for internal/web to serve.
package static

import "embed"

//go:embed *.css *.js
var FS embed.FS

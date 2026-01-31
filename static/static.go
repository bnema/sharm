package static

import "embed"

//go:embed *.js *.svg
var FS embed.FS

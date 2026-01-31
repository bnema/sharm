package static

import "embed"

//go:embed *.js *.svg *.png *.ico
var FS embed.FS

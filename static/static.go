package static

import "embed"

//go:embed *.js *.svg *.png *.ico *.json *.webmanifest
var FS embed.FS

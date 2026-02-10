package static

import "embed"

//go:embed *.js *.svg *.png *.ico *.json
var FS embed.FS

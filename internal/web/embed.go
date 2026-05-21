package web

import "embed"

//go:embed templates/*.html static/*.css
var Assets embed.FS

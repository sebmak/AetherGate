package web

import "embed"

// DistFS contains the compiled React single page application assets
//
//go:embed all:dist
var DistFS embed.FS

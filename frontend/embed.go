package frontend

import "embed"

// Dist contains the embedded frontend distribution files.
//
//go:embed all:dist
var Dist embed.FS

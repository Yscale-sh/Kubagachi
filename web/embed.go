// Package web embeds the built browser UI (the vite build output in dist/)
// so the kubagachi binary can serve it without any files on disk.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS

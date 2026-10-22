// Package frontend embeds the local UI into the binary.
package frontend

import "embed"

//go:embed index.html app.css app.js logo.svg
var Files embed.FS

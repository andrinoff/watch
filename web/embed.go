// Package web embeds the built frontend so the server ships as one binary.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the compiled Vite build (web/dist). Build it with `make ui`
// before compiling; the placeholder file keeps this compiling until then.
//
//go:embed all:dist
var dist embed.FS

// FS returns the embedded frontend rooted at dist/.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}

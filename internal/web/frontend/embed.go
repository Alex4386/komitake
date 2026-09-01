// Package frontend embeds the built Vite frontend. The dist directory is
// populated by `npm run build`; when it contains only the placeholder, Dist
// reports that no build is present.
package frontend

import (
	"embed"
	"errors"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// ErrNotBuilt is returned by Dist when the frontend has not been built.
var ErrNotBuilt = errors.New("frontend not built")

// Dist returns the built frontend as a filesystem rooted at the dist dir. It
// returns ErrNotBuilt when no index.html is present.
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, ErrNotBuilt
	}
	return sub, nil
}

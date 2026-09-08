// Package media serves the pictures the demo data points at.
//
// The seed used to leave every image column NULL, so half the app rendered
// grey boxes, and the alternative — pointing at a stock-photo CDN — makes the
// demo depend on somebody else's uptime and quietly break offline. The images
// live in this repository instead, are embedded in the binary, and are served
// from the API under /api/media, which is an origin both apps already reach.
//
// They are SVG on purpose: a few hundred bytes each, sharp at any size, and
// diffable. Regenerate them with `go run ./internal/media/gen`.
package media

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets/*.svg
var assets embed.FS

// FS is the embedded image set, for anything that wants to read it directly.
func FS() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// The embed directive above guarantees the directory exists; a failure
		// here would mean the binary was built wrong.
		panic("media: assets directory missing from the binary: " + err.Error())
	}
	return sub
}

// URL is where an image is served from, as it is stored in the database.
//
// A path rather than an absolute URL: the apps call /api on their own origin,
// so the same value works behind nginx, behind a dev proxy, and in a browser
// pointed straight at the API.
func URL(name string) string { return "/api/media/" + name }

// Handler serves the embedded images.
//
// They are immutable — a change to a picture is a change to its file name — so
// they are cached hard. The demo's first paint is the only time anything here
// crosses the network.
func Handler() http.Handler {
	files := http.FileServer(http.FS(FS()))
	return http.StripPrefix("/api/media/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".svg") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Type", "image/svg+xml")
		files.ServeHTTP(w, r)
	}))
}

// Package brand holds the Earthler mark and the glyphs and icons of the
// four apps, and serves them. These files are not under the code license:
// see TRADEMARKS.md. A modified version of the system must replace them.
package brand

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed earthler-mark.png earthler-wordmark.png apps
var files embed.FS

// Glyphs are the templates that draw the glyph of each app, as glyph-talk,
// glyph-learn, glyph-decide and glyph-ask.
//
//go:embed apps.html
var Glyphs string

// Handler serves the files at /brand/, for a long time, because they only
// change with a new version of the system.
func Handler() http.Handler {
	static, err := fs.Sub(files, ".")
	if err != nil {
		panic(err)
	}
	server := http.StripPrefix("/brand/", http.FileServerFS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // files, never a listing of a folder
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		server.ServeHTTP(w, r)
	})
}

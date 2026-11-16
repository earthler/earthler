// Package decide is Decide, the app of proposals, votes and the map. It is not built yet:
// its home says so, in the same face as every app, with its bar.
package decide

import (
	"embed"
	"net/http"

	"github.com/earthler/earthler/core/page"
)

//go:embed decide.html
var files embed.FS

// Paths is what the screen serves of Decide's bar.
var Paths = []string{"/decide/"}

// Routes adds Decide's home. who says who is signed in.
func Routes(mux *http.ServeMux, pages *page.Renderer, signedIn func(http.Handler) http.Handler, who page.Who) {
	tmpl := pages.Screen(files, "decide.html")
	mux.Handle("GET /decide/{$}", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, role := who(r)
		pages.Render(w, http.StatusOK, tmpl, page.Data{Title: pages.T("app.decide.tab.decisions"), App: "decide", Tab: "decisions", User: name, Role: role})
	})))
}

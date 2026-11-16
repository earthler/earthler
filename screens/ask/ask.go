// Package ask is Ask, the app of questions to the Mentor. It is not built yet:
// its home says so, in the same face as every app, with its bar.
package ask

import (
	"embed"
	"net/http"

	"github.com/earthler/earthler/core/page"
)

//go:embed ask.html
var files embed.FS

// Paths is what the screen serves of Ask's bar.
var Paths = []string{"/ask/"}

// Routes adds Ask's home. who says who is signed in.
func Routes(mux *http.ServeMux, pages *page.Renderer, signedIn func(http.Handler) http.Handler, who page.Who) {
	tmpl := pages.Screen(files, "ask.html")
	mux.Handle("GET /ask/{$}", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, role := who(r)
		pages.Render(w, http.StatusOK, tmpl, page.Data{Title: pages.T("app.ask.tab.ask"), App: "ask", Tab: "ask", User: name, Role: role})
	})))
}

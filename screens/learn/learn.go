// Package learn is Learn, the app of lessons, the wiki, the collection and
// films. None of them is built yet: its home has the search field, which
// already finds what the community wrote, and says what is still to come.
package learn

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/earthler/earthler/core/page"
)

//go:embed learn.html
var files embed.FS

// Paths is what the screen serves of Learn's bar.
var Paths = []string{"/learn/"}

// Routes adds Learn's home. who says who is signed in.
func Routes(mux *http.ServeMux, pages *page.Renderer, signedIn func(http.Handler) http.Handler, who page.Who) {
	tmpl := pages.Screen(files, "learn.html")
	mux.Handle("GET /learn/{$}", signedIn(home(pages, tmpl, who)))
}

func home(pages *page.Renderer, tmpl *template.Template, who page.Who) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, role := who(r)
		pages.Render(w, http.StatusOK, tmpl, page.Data{Title: pages.T("app.learn.tab.subjects"), App: "learn", Tab: "subjects", User: name, Role: role})
	})
}

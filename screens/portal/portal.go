// Package portal is the community's own page: what its address opens. It
// is the center of everything, with one field that searches the whole
// community and the four apps around it, each with the button that puts it
// on the phone. The portal itself is not an app and never becomes an icon:
// the four apps are, each on a path of its own.
package portal

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/page"
)

//go:embed portal.html
var files embed.FS

// Screen serves the portal.
type Screen struct {
	accounts *accounts.Service
	pages    *page.Renderer
	tmpl     *template.Template
}

func New(a *accounts.Service, pages *page.Renderer) *Screen {
	return &Screen{accounts: a, pages: pages, tmpl: pages.Screen(files, "portal.html")}
}

// Routes adds the portal. Someone who has no account yet is sent to create
// one first, because on an iPhone an app added before signing in would
// have to be signed in apart.
func (s *Screen) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		sess, signedIn := s.accounts.Session(r)
		if !signedIn {
			http.Redirect(w, r, "/sign-up", http.StatusSeeOther)
			return
		}
		s.pages.Render(w, http.StatusOK, s.tmpl, page.Data{
			Title: s.pages.T("portal.title"), User: sess.Name, Role: string(sess.Role), Body: page.Apps})
	})
}

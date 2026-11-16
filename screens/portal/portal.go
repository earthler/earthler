// Package portal is the community's own page: what its address opens. It
// is the center of everything, with one field that searches the whole
// community, the five apps, and what is happening now. The portal itself is
// not an app and never becomes an icon: the five apps are, each on a path of
// its own. It is drawn in docs/screens/src/portal/portal.html and described
// in docs/product/portal.md.
package portal

import (
	"embed"
	"html/template"
	"net/http"
	"strings"
	"unicode"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/page"
)

//go:embed portal.html profile.html
var files embed.FS

// Screen serves the portal and the page the person's initial leads to.
type Screen struct {
	accounts *accounts.Service
	pages    *page.Renderer
	address  string // the community's address, the same on every machine
	portal   *template.Template
	profile  *template.Template
}

// New prepares the portal of the community reached at address.
func New(a *accounts.Service, pages *page.Renderer, address string) *Screen {
	return &Screen{
		accounts: a, pages: pages, address: address,
		portal:  pages.Screen(files, "portal.html"),
		profile: pages.Screen(files, "profile.html"),
	}
}

// Routes adds the portal and the account page. Someone who has no account
// yet is sent to create one first, because on an iPhone an app added before
// signing in would have to be signed in apart.
func (s *Screen) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.show)
	mux.HandleFunc("GET /profile", s.showProfile)
}

// happening is one line of "Agora na comunidade": an alert of Talk, an open
// vote of Decide, a new lesson of Learn, a new place of Maps. No app writes
// these yet, so the portal shows that nothing is new.
type happening struct {
	App   string // the app it comes from, for its small tile
	Title string
	Line  string // what it is and when, such as "Aviso no Talk, há 2 horas"
	Link  string // where it opens, in its app
}

type body struct {
	Address string
	Initial string
	Account string // what a screen reader says of the initial
	Now     []happening
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request) {
	sess, signedIn := s.accounts.Session(r)
	if !signedIn {
		http.Redirect(w, r, "/sign-up", http.StatusSeeOther)
		return
	}
	s.pages.Render(w, http.StatusOK, s.portal, page.Data{
		Title: s.pages.T("portal.title"), User: sess.Name, Role: string(sess.Role),
		Bare: true, Script: "/static/portal.js",
		Body: body{
			Address: s.address,
			Initial: initial(sess.Name),
			Account: strings.ReplaceAll(s.pages.T("portal.account"), "{name}", sess.Name),
		},
	})
}

type profileBody struct {
	Initial, Name, Role string
}

// showProfile is the account, as far as it is built: who the person is.
// The photo, the vault, the devices and the trusted people come later.
func (s *Screen) showProfile(w http.ResponseWriter, r *http.Request) {
	sess, signedIn := s.accounts.Session(r)
	if !signedIn {
		http.Redirect(w, r, "/sign-up", http.StatusSeeOther)
		return
	}
	title := s.pages.T("profile.title")
	s.pages.Render(w, http.StatusOK, s.profile, page.Data{
		Title: title, User: sess.Name, Role: string(sess.Role),
		Inner: &page.Inner{Title: title, Sub: s.pages.Community(), Back: "/"},
		Body:  profileBody{Initial: initial(sess.Name), Name: sess.Name, Role: s.pages.T("role." + string(sess.Role))},
	})
}

// initial is the first letter of a name, as the round button shows it.
func initial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return string(unicode.ToUpper(r))
	}
	return "?"
}

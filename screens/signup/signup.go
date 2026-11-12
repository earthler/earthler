// Package signup is the screen where a resident creates an account.
package signup

import (
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/scribe"
)

//go:embed signup.html
var files embed.FS

// Screen serves the sign up.
type Screen struct {
	accounts *accounts.Service
	pages    *page.Renderer
	tmpl     *template.Template
}

func New(a *accounts.Service, pages *page.Renderer) *Screen {
	return &Screen{accounts: a, pages: pages, tmpl: pages.Screen(files, "signup.html")}
}

// Routes adds the screen to the server.
func (s *Screen) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /sign-up", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.accounts.Session(r); ok {
			http.Redirect(w, r, "/chat", http.StatusSeeOther)
			return
		}
		s.show(w, http.StatusOK, "", "")
	})
	mux.HandleFunc("POST /sign-up", s.post)
}

type body struct {
	Error   string
	Name    string
	MaxName int
}

func (s *Screen) show(w http.ResponseWriter, status int, errKey, name string) {
	b := body{Name: name, MaxName: accounts.MaxName}
	if errKey != "" {
		b.Error = s.pages.T(errKey)
	}
	s.pages.Render(w, status, s.tmpl, page.Data{Title: s.pages.T("signup.title"), Body: b})
}

func (s *Screen) post(w http.ResponseWriter, r *http.Request) {
	if !page.SameOrigin(r) {
		http.Error(w, s.pages.T("error.origin"), http.StatusForbidden)
		return
	}
	name := r.FormValue("name")
	cookie, err := s.accounts.SignUp(r.Context(), name, r.TLS != nil)
	switch {
	case errors.Is(err, accounts.ErrNameEmpty):
		s.show(w, http.StatusBadRequest, "signup.error.empty", "")
	case errors.Is(err, accounts.ErrNameLong):
		s.show(w, http.StatusBadRequest, "signup.error.long", name)
	case errors.Is(err, accounts.ErrNameInvalid):
		s.show(w, http.StatusBadRequest, "signup.error.invalid", "")
	case errors.Is(err, accounts.ErrNameTaken):
		s.show(w, http.StatusConflict, "signup.error.taken", name)
	case errors.Is(err, scribe.ErrStopped):
		s.show(w, http.StatusServiceUnavailable, "error.stopped", name)
	case err != nil:
		log.Printf("signup: %v", err)
		s.show(w, http.StatusInternalServerError, "error.internal", name)
	default:
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/chat", http.StatusSeeOther)
	}
}

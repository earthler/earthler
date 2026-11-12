// Package page is the shell that every screen is drawn in: the layout, the
// base style and the texts in the community's language.
package page

import (
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"

	"github.com/earthler/earthler/locales"
)

//go:embed layout.html static
var files embed.FS

// Renderer draws screens inside the layout.
type Renderer struct {
	lang      string
	community string
	texts     locales.Catalog
	base      *template.Template
}

// Data is what a screen hands to the layout.
type Data struct {
	Title  string // title of the screen, already in the community's language
	Active string // which item of the navigation is this screen
	User   string // name of the person, or empty when nobody is signed in
	Query  string // what is in the search field
	Body   any    // whatever the screen's own template needs
	Script string // address of the screen's own script, for the few that need one
	Images string // another origin that images may be loaded from, or empty

	// Filled by the renderer.
	Lang      string
	Community string
}

// New prepares the layout for a language and a community name.
func New(lang, community string) (*Renderer, error) {
	texts, err := locales.Load(lang)
	if err != nil {
		return nil, err
	}
	r := &Renderer{lang: lang, community: community, texts: texts}
	r.base, err = template.New("layout.html").Funcs(template.FuncMap{"t": texts.T}).ParseFS(files, "layout.html")
	if err != nil {
		return nil, err
	}
	return r, nil
}

// T returns a screen text in the community's language.
func (r *Renderer) T(key string) string { return r.texts.T(key) }

// Community is the name of the community, for screens that write it into
// something other than the layout.
func (r *Renderer) Community() string { return r.community }

// Screen adds a screen's templates to a copy of the layout. One of the
// files must define a template called "content".
func (r *Renderer) Screen(fsys fs.FS, files ...string) *template.Template {
	base := template.Must(r.base.Clone())
	return template.Must(base.ParseFS(fsys, files...))
}

// Render writes a screen.
func (r *Renderer) Render(w http.ResponseWriter, status int, screen *template.Template, data Data) {
	data.Lang, data.Community = r.lang, r.community
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	policy := "default-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'"
	if data.Images != "" {
		policy += "; img-src 'self' " + data.Images
	}
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := screen.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("page: %v", err)
	}
}

// Static serves the base style.
func Static() http.Handler {
	static, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServerFS(static))
}

// SameOrigin reports whether a request that changes something came from one
// of the system's own pages. Browsers send Origin with every form they post,
// so a request without it, or with another site in it, is refused. The
// scheme counts too: a page served over plain HTTP, which anyone on the
// network could have changed on the way, is not the secure site.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Scheme == scheme && u.Host == r.Host
}

// MaxForm is the most a form may send, in bytes.
const MaxForm = 64 << 10

// Guard stands in front of every screen. A request that changes something
// is read here, once, with a limit, so no screen can forget it. Only forms
// that are sent as plain text are read: anything else, a file for instance,
// arrives as an empty form and is refused by the screen, and its body is
// never read or kept on the disk.
func (r *Renderer) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, req)
			return
		}
		if req.ContentLength > MaxForm {
			http.Error(w, r.T("error.toolarge"), http.StatusRequestEntityTooLarge)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, MaxForm)
		if err := req.ParseForm(); err != nil {
			status := http.StatusBadRequest
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				status = http.StatusRequestEntityTooLarge
			}
			http.Error(w, r.T("error.toolarge"), status)
			return
		}
		next.ServeHTTP(w, req)
	})
}

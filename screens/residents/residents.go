// Package residents is the screen where an administrator sees everyone who
// lives in the community and gives them roles.
package residents

import (
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/permissions"
	"github.com/earthler/earthler/core/scribe"
)

//go:embed residents.html
var files embed.FS

// Screen serves the list of residents.
type Screen struct {
	accounts *accounts.Service
	pages    *page.Renderer
	tmpl     *template.Template
}

func New(a *accounts.Service, pages *page.Renderer) *Screen {
	return &Screen{accounts: a, pages: pages, tmpl: pages.Screen(files, "residents.html")}
}

// Routes adds the screen to the server.
func (s *Screen) Routes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/residents", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.show(w, r, http.StatusOK, "")
	})))
	mux.Handle("POST /admin/residents/role", signedIn(http.HandlerFunc(s.changeRole)))
}

type option struct {
	Value    string
	Label    string
	Selected bool
}

type line struct {
	Number  uint64
	Name    string
	Role    string // the name of the role, in the community's language
	Joined  string
	Options []option // empty when the role cannot be changed by hand
}

type body struct {
	Residents []line
	Error     string
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request, status int, errKey string) {
	sess := accounts.From(r.Context())
	if !permissions.Can(sess.Role, permissions.SeeResidents) {
		http.Error(w, s.pages.T("error.forbidden"), http.StatusForbidden)
		return
	}
	all, err := s.accounts.Residents(r.Context())
	if err != nil {
		log.Printf("residents: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	var b body
	if errKey != "" {
		b.Error = s.pages.T(errKey)
	}
	canChange := permissions.Can(sess.Role, permissions.ChangeRoles)
	for _, resident := range all {
		l := line{
			Number: resident.Number,
			Name:   resident.Name,
			Role:   s.pages.T("role." + string(resident.Role)),
			Joined: resident.Joined.Format(s.pages.T("time.short")),
		}
		if canChange && resident.Role != permissions.Child {
			for _, role := range permissions.Roles {
				if permissions.Assignable(role) {
					l.Options = append(l.Options, option{Value: string(role), Label: s.pages.T("role." + string(role)), Selected: role == resident.Role})
				}
			}
		}
		b.Residents = append(b.Residents, l)
	}
	s.pages.Render(w, status, s.tmpl, page.Data{
		Title: s.pages.T("residents.title"), Active: "residents", User: sess.Name, Role: string(sess.Role), Body: b})
}

func (s *Screen) changeRole(w http.ResponseWriter, r *http.Request) {
	if !page.SameOrigin(r) {
		http.Error(w, s.pages.T("error.origin"), http.StatusForbidden)
		return
	}
	sess := accounts.From(r.Context())
	target, err := strconv.ParseUint(r.FormValue("number"), 10, 63)
	if err != nil || target == 0 {
		http.NotFound(w, r)
		return
	}
	err = s.accounts.ChangeRole(r.Context(), sess, target, permissions.Role(r.FormValue("role")))
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/residents", http.StatusSeeOther)
	case errors.Is(err, accounts.ErrNotAllowed):
		http.Error(w, s.pages.T("error.forbidden"), http.StatusForbidden)
	case errors.Is(err, accounts.ErrNoSuchAccount):
		http.NotFound(w, r)
	case errors.Is(err, accounts.ErrBadRole):
		s.show(w, r, http.StatusBadRequest, "residents.error.role")
	case errors.Is(err, accounts.ErrLastAdministrator):
		s.show(w, r, http.StatusConflict, "residents.error.last")
	case errors.Is(err, scribe.ErrStopped):
		s.show(w, r, http.StatusServiceUnavailable, "error.stopped")
	default:
		log.Printf("residents: %v", err)
		s.show(w, r, http.StatusInternalServerError, "error.internal")
	}
}

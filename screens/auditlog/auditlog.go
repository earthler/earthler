// Package auditlog is the screen that shows what the people who have powers
// did with them: who gave which role, and which message was removed and why.
// It is open to every resident, so a power that is used is a power that is
// seen.
package auditlog

import (
	"database/sql"
	"embed"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/permissions"
	"github.com/earthler/earthler/core/roster"
)

//go:embed log.html
var files embed.FS

// Area is the index of acts. names keeps the name each account had when it
// joined, so the screen never reads another index.
var Area = index.Area{
	Name: "auditlog",
	Schema: `
		CREATE TABLE names (account INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE acts (number INTEGER PRIMARY KEY, time INTEGER NOT NULL, actor INTEGER NOT NULL,
			kind TEXT NOT NULL, target INTEGER NOT NULL, detail TEXT NOT NULL);
		` + roster.Schema,
	Apply: apply,
}

// The kinds of act the log shows.
const (
	kindRole    = "role"
	kindRemoval = "removal"
)

func apply(tx *sql.Tx, entry journal.Entry) error {
	changed, err := roster.Apply(tx, entry)
	if err != nil {
		return err
	}
	e, err := event.Decode(entry.Content)
	if err != nil {
		return nil
	}
	switch e.Type {
	case event.AccountCreated:
		var d event.AccountCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT OR IGNORE INTO names (account, name) VALUES (?, ?)`, entry.Number, d.Name)
	case event.RoleChanged:
		// Only a change that took effect is an act: the roster knows who was
		// an administrator at the time, so a forged entry never shows here.
		var d event.RoleChangedData
		if !changed || e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO acts (number, time, actor, kind, target, detail) VALUES (?, ?, ?, ?, ?, ?)`,
			entry.Number, e.Time, e.Account, kindRole, d.Account, d.Role)
	case event.MessageDeleted:
		var d event.MessageDeletedData
		// Authors delete without a reason. Only the removal of someone
		// else's message carries one.
		if e.Read(&d) != nil || d.Reason == "" {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO acts (number, time, actor, kind, target, detail) VALUES (?, ?, ?, ?, ?, ?)`,
			entry.Number, e.Time, e.Account, kindRemoval, d.Author, d.Reason)
	}
	return err
}

// Screen serves the log.
type Screen struct {
	index *index.Index
	pages *page.Renderer
	tmpl  *template.Template
}

func New(ix *index.Index, pages *page.Renderer) *Screen {
	return &Screen{index: ix, pages: pages, tmpl: pages.Screen(files, "log.html")}
}

// Routes adds the screen to the server.
func (s *Screen) Routes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler) {
	mux.Handle("GET /log", signedIn(http.HandlerFunc(s.show)))
}

// MaxActs is how many of the latest acts the screen shows.
const MaxActs = 100

type act struct {
	Time string
	Text string
}

type body struct {
	Acts []act
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request) {
	sess := accounts.From(r.Context())
	if !permissions.Can(sess.Role, permissions.ReadTheLog) {
		http.Error(w, s.pages.T("error.forbidden"), http.StatusForbidden)
		return
	}
	rows, err := s.index.DB().QueryContext(r.Context(), `
		SELECT a.time, coalesce(who.name, ''), coalesce(whom.name, ''), a.kind, a.detail FROM acts a
		LEFT JOIN names who ON who.account = a.actor
		LEFT JOIN names whom ON whom.account = a.target
		ORDER BY a.number DESC LIMIT ?`, MaxActs)
	if err != nil {
		log.Printf("auditlog: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var b body
	for rows.Next() {
		var ms int64
		var actor, target, kind, detail string
		if err := rows.Scan(&ms, &actor, &target, &kind, &detail); err != nil {
			log.Printf("auditlog: %v", err)
			continue
		}
		b.Acts = append(b.Acts, act{Time: time.UnixMilli(ms).Format(s.pages.T("time.short")), Text: s.sentence(kind, actor, target, detail)})
	}
	s.pages.Render(w, http.StatusOK, s.tmpl, page.Data{
		Title: s.pages.T("log.title"), Active: "log", User: sess.Name, Role: string(sess.Role), Body: b})
}

// sentence says an act in the community's language.
func (s *Screen) sentence(kind, actor, target, detail string) string {
	switch kind {
	case kindRole:
		return strings.NewReplacer("{actor}", actor, "{target}", target, "{role}", s.pages.T("role."+detail)).Replace(s.pages.T("log.role"))
	case kindRemoval:
		return strings.NewReplacer("{actor}", actor, "{target}", target, "{reason}", detail).Replace(s.pages.T("log.removal"))
	}
	return ""
}

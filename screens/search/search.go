// Package search is the search screen: one field that looks in everything
// the community wrote and says where each result came from.
package search

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
	terms "github.com/earthler/earthler/core/search"
)

//go:embed search.html
var files embed.FS

// Area is the search index. The terms table holds each text already reduced
// to its search terms, and docs holds what is shown in the results. Nothing
// personal ever enters this index.
var Area = index.Area{
	Name: "search",
	Schema: `
		CREATE VIRTUAL TABLE terms USING fts5(body);
		INSERT INTO terms (terms, rank) VALUES ('secure-delete', 1);
		CREATE TABLE docs (
			number INTEGER PRIMARY KEY, kind TEXT NOT NULL, account INTEGER NOT NULL,
			time INTEGER NOT NULL, text TEXT NOT NULL);
		CREATE TABLE authors (account INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
	Apply: apply,
}

func apply(tx *sql.Tx, entry journal.Entry) error {
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
		_, err = tx.Exec(`INSERT INTO authors (account, name) VALUES (?, ?)`, entry.Number, d.Name)
	case event.MessageCreated:
		var d event.MessageCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		if _, err = tx.Exec(`INSERT INTO docs (number, kind, account, time, text) VALUES (?, 'chat', ?, ?, ?)`,
			entry.Number, e.Account, e.Time, d.Text); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO terms (rowid, body) VALUES (?, ?)`, entry.Number, strings.Join(terms.Terms(d.Text), " "))
	case event.MessageDeleted:
		var d event.MessageDeletedData
		if e.Read(&d) != nil {
			return nil
		}
		if _, err = tx.Exec(`DELETE FROM docs WHERE number = ?`, d.Message); err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM terms WHERE rowid = ?`, d.Message)
	}
	return err
}

// Screen serves the search.
type Screen struct {
	index *index.Index
	pages *page.Renderer
	tmpl  *template.Template
}

func New(ix *index.Index, pages *page.Renderer) *Screen {
	return &Screen{index: ix, pages: pages, tmpl: pages.Screen(files, "search.html")}
}

// Routes adds the screen to the server.
func (s *Screen) Routes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler) {
	mux.Handle("GET /search", signedIn(http.HandlerFunc(s.show)))
}

type result struct {
	Origin string
	Author string
	Time   string
	Text   string
}

type body struct {
	Asked   bool
	Results []result
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request) {
	sess := accounts.From(r.Context())
	asked := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(asked)) > 200 {
		asked = string([]rune(asked)[:200])
	}
	b := body{}
	if query := terms.Query(asked); query != "" {
		b.Asked = true
		rows, err := s.index.DB().QueryContext(r.Context(), `
			SELECT d.kind, coalesce(a.name, ''), d.time, d.text FROM terms
			JOIN docs d ON d.number = terms.rowid
			LEFT JOIN authors a ON a.account = d.account
			WHERE terms MATCH ? ORDER BY terms.rank LIMIT 50`, query)
		if err != nil {
			log.Printf("search: %v", err)
			http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		layout := s.pages.T("time.short")
		for rows.Next() {
			var kind string
			var ms int64
			var res result
			if err := rows.Scan(&kind, &res.Author, &ms, &res.Text); err != nil {
				log.Printf("search: %v", err)
				continue
			}
			res.Origin = s.pages.T("search.origin." + kind)
			res.Time = time.UnixMilli(ms).Format(layout)
			if text := []rune(res.Text); len(text) > 400 {
				res.Text = string(text[:400]) + "…"
			}
			b.Results = append(b.Results, res)
		}
	}
	s.pages.Render(w, http.StatusOK, s.tmpl, page.Data{Title: s.pages.T("search.title"), User: sess.Name, Query: asked, Body: b})
}

// Package search is the search screen: one field that looks in everything
// the community wrote and says where each result came from. It is drawn in
// docs/screens/src/portal/search.html and search-empty.html, and described
// in docs/product/portal.md. Today only Talk writes what can be found: the
// messages of the open channels.
package search

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/page"
	terms "github.com/earthler/earthler/core/search"
)

//go:embed search.html
var files embed.FS

// Area is the search index. The terms table holds each text already reduced
// to its search terms, and docs holds what is shown in the results. gone
// remembers what was deleted, because a deletion written on one machine can
// arrive before the message it deletes, which was written on another. Nothing
// personal ever enters this index.
var Area = index.Area{
	Name: "search",
	Schema: `
		CREATE VIRTUAL TABLE terms USING fts5(body);
		INSERT INTO terms (terms, rank) VALUES ('secure-delete', 1);
		CREATE TABLE docs (
			number INTEGER PRIMARY KEY, kind TEXT NOT NULL, account INTEGER NOT NULL,
			channel INTEGER NOT NULL, time INTEGER NOT NULL, text TEXT NOT NULL);
		CREATE TABLE authors (account INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE channels (number INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE gone (number INTEGER PRIMARY KEY);`,
	Apply: apply,
}

func apply(tx *sql.Tx, entry index.Entry) error {
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
		_, err = tx.Exec(`INSERT OR IGNORE INTO authors (account, name) VALUES (?, ?)`, entry.ID, d.Name)
	case event.ChannelCreated:
		var d event.ChannelCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT OR IGNORE INTO channels (number, name) VALUES (?, ?)`, entry.ID, d.Name)
	case event.MessageCreated:
		var d event.MessageCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		var gone int
		if err = tx.QueryRow(`SELECT count(*) FROM gone WHERE number = ?`, entry.ID).Scan(&gone); err != nil || gone > 0 {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO docs (number, kind, account, channel, time, text) VALUES (?, 'chat', ?, ?, ?, ?)`,
			entry.ID, e.Account, d.Channel, e.Time, d.Text); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO terms (rowid, body) VALUES (?, ?)`, entry.ID, strings.Join(terms.Terms(d.Text), " "))
	case event.MessageDeleted:
		var d event.MessageDeletedData
		if e.Read(&d) != nil {
			return nil
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO gone (number) VALUES (?)`, d.Message); err != nil {
			return err
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

// Routes adds the search of the whole community, at /search, and the same
// search from inside Learn, which keeps the resident in Learn.
func (s *Screen) Routes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler) {
	mux.Handle("GET /search", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.show(w, r, "", "/search")
	})))
	mux.Handle("GET /learn/search", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.show(w, r, "learn", "/learn/search")
	})))
}

// filters are the apps a search can be narrowed to, in the order the row
// shows them. The first, with no key, is everything.
var filters = []string{"", "talk", "learn", "decide", "ask", "maps"}

// searched says which filters find something today: only Talk writes
// what the search can find.
func searched(in string) bool { return in == "" || in == "talk" }

const most = 50 // results on one page, the best first

type filter struct {
	Label string
	Link  string
	On    bool
}

type result struct {
	App  string        // the app it comes from, for its small tile
	From string        // the app and where in it, such as "Talk, Conversa geral"
	Text template.HTML // the text, with the words found marked
	When string        // when it was written, in the words people use
	Link string        // where it opens, in its app
}

type body struct {
	Action  string // where the field sends the next search
	Asked   bool   // something was typed
	Filters []filter
	Found   string // how many results, and where
	Results []result
	NoneIn  string // nothing in the app chosen
	AllLink string // the same words in every app
	Nothing string // nothing found anywhere
	AskLink string // the same words, to the Mentor
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request, app, action string) {
	sess := accounts.From(r.Context())
	asked := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(asked)) > 200 {
		asked = string([]rune(asked)[:200])
	}
	in := r.URL.Query().Get("in")
	if !known(in) || app != "" {
		in = ""
	}
	query := terms.Query(asked)
	b := body{Action: action, Asked: query != ""}
	if app == "" {
		for _, key := range filters {
			b.Filters = append(b.Filters, filter{Label: s.pages.T("search.in." + name(key)), Link: link(asked, key), On: key == in})
		}
	}
	if b.Asked && !searched(in) {
		b.NoneIn = s.fill("search.none.in", "{in}", s.pages.T("search.in."+in), "{q}", asked)
		b.AllLink = link(asked, "")
	}
	if b.Asked && searched(in) {
		var err error
		if b.Results, err = s.find(r, query, asked); err != nil {
			log.Printf("search: %v", err)
			http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
			return
		}
		b.Found = s.found(len(b.Results), in)
		if len(b.Results) == 0 {
			b.Nothing = s.fill("search.nothing", "{q}", asked)
			b.AskLink = "/ask/?q=" + url.QueryEscape(asked)
		}
	}
	data := page.Data{Title: s.pages.T("search.title"), User: sess.Name, Role: string(sess.Role), Query: asked, Body: b}
	if app != "" {
		data.App, data.Tab = app, "subjects"
	} else {
		data.Bare = true
	}
	s.pages.Render(w, http.StatusOK, s.tmpl, data)
}

// find looks for the words in the open channels, the best first.
func (s *Screen) find(r *http.Request, query, asked string) ([]result, error) {
	rows, err := s.index.DB().QueryContext(r.Context(), `
		SELECT d.number, d.channel, coalesce(c.name, ''), coalesce(a.name, ''), d.time, d.text FROM terms
		JOIN docs d ON d.number = terms.rowid
		LEFT JOIN channels c ON c.number = d.channel
		LEFT JOIN authors a ON a.account = d.account
		WHERE terms MATCH ? ORDER BY terms.rank LIMIT ?`, query, most)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roots := map[string]bool{}
	for _, t := range terms.Terms(asked) {
		roots[t] = true
	}
	now := time.Now()
	var out []result
	for rows.Next() {
		var number, channel uint64
		var where, author, text string
		var ms int64
		if err := rows.Scan(&number, &channel, &where, &author, &ms, &text); err != nil {
			return nil, err
		}
		shown := marked(excerpt(text, roots, 400), roots)
		if author != "" {
			shown = template.HTML(template.HTMLEscapeString(author)+": ") + shown
		}
		out = append(out, result{
			App:  "talk",
			From: s.pages.T("app.talk.name") + ", " + where,
			Text: shown,
			When: s.fill("search.talk.when", "{when}", s.pages.When(time.UnixMilli(ms), now)),
			Link: fmt.Sprintf("/talk/channel/%d#m%d", channel, number),
		})
	}
	return out, rows.Err()
}

// found says how many results there are, and where they were looked for.
func (s *Screen) found(n int, in string) string {
	key := "search.found"
	switch {
	case n == most:
		key += ".most"
	case n == 1:
		key += ".one"
	}
	where := s.pages.T("search.everywhere")
	if in != "" {
		where = s.fill("search.where", "{in}", s.pages.T("search.in."+in))
	}
	return s.fill(key, "{n}", strconv.Itoa(n), "{where}", where)
}

func (s *Screen) fill(key string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(s.pages.T(key))
}

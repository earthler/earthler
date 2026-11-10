// Package chat is the conversation screen. For now it shows one open channel
// that every resident reads and writes.
package chat

import (
	"database/sql"
	"embed"
	"errors"
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
	"github.com/earthler/earthler/core/scribe"
)

//go:embed chat.html
var files embed.FS

// MaxText is the longest message, in characters.
const MaxText = 4000

// Area is the index this screen reads.
var Area = index.Area{
	Name: "chat",
	Schema: `
		CREATE TABLE channels (number INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE authors (account INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE messages (
			number INTEGER PRIMARY KEY, channel INTEGER NOT NULL, account INTEGER NOT NULL,
			time INTEGER NOT NULL, text TEXT NOT NULL);
		CREATE INDEX messages_by_channel ON messages (channel, number);`,
	Apply: apply,
}

func apply(tx *sql.Tx, entry journal.Entry) error {
	e, err := event.Decode(entry.Content)
	if err != nil {
		return nil
	}
	switch e.Type {
	case event.ChannelCreated:
		var d event.ChannelCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO channels (number, name) VALUES (?, ?)`, entry.Number, d.Name)
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
		_, err = tx.Exec(`INSERT INTO messages (number, channel, account, time, text) VALUES (?, ?, ?, ?, ?)`,
			entry.Number, d.Channel, e.Account, e.Time, d.Text)
	}
	return err
}

// Screen serves the conversation.
type Screen struct {
	index  *index.Index
	scribe *scribe.Scribe
	pages  *page.Renderer
	tmpl   *template.Template
}

func New(ix *index.Index, s *scribe.Scribe, pages *page.Renderer) *Screen {
	return &Screen{index: ix, scribe: s, pages: pages, tmpl: pages.Screen(files, "chat.html")}
}

// Routes adds the screen to the server. signedIn wraps what only residents
// may reach.
func (s *Screen) Routes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler) {
	mux.Handle("GET /chat", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.show(w, r, http.StatusOK, "", "")
	})))
	mux.Handle("POST /chat", signedIn(http.HandlerFunc(s.post)))
}

type message struct {
	Author string
	Time   string
	Text   string
	Mine   bool
}

type body struct {
	Channel  string
	Messages []message
	Error    string // key of a text to show, or empty
	Draft    string // what the person had written when an error happened
	MaxText  int
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request, status int, errKey, draft string) {
	sess := accounts.From(r.Context())
	b := body{Draft: draft, MaxText: MaxText}
	if errKey != "" {
		b.Error = s.pages.T(errKey)
	}
	channel, name, err := s.channel(r)
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	b.Channel = name

	// The last messages, shown oldest first.
	rows, err := s.index.DB().QueryContext(r.Context(), `
		SELECT m.account, coalesce(a.name, ''), m.time, m.text FROM messages m
		LEFT JOIN authors a ON a.account = m.account
		WHERE m.channel = ? ORDER BY m.number DESC LIMIT 200`, channel)
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	layout := s.pages.T("time.short")
	for rows.Next() {
		var account uint64
		var ms int64
		var m message
		if err := rows.Scan(&account, &m.Author, &ms, &m.Text); err != nil {
			log.Printf("chat: %v", err)
			continue
		}
		m.Time = time.UnixMilli(ms).Format(layout)
		m.Mine = account == sess.Account
		b.Messages = append(b.Messages, m)
	}
	for i, j := 0, len(b.Messages)-1; i < j; i, j = i+1, j-1 {
		b.Messages[i], b.Messages[j] = b.Messages[j], b.Messages[i]
	}
	s.pages.Render(w, status, s.tmpl, page.Data{Title: s.pages.T("chat.title"), Active: "chat", User: sess.Name, Body: b})
}

// channel returns the open channel. There is one for now, created together
// with the community.
func (s *Screen) channel(r *http.Request) (uint64, string, error) {
	var number uint64
	var name string
	err := s.index.DB().QueryRowContext(r.Context(), `SELECT number, name FROM channels ORDER BY number LIMIT 1`).Scan(&number, &name)
	return number, name, err
}

func (s *Screen) post(w http.ResponseWriter, r *http.Request) {
	if !page.SameOrigin(r) {
		http.Error(w, s.pages.T("error.origin"), http.StatusForbidden)
		return
	}
	sess := accounts.From(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	text := strings.TrimSpace(strings.ReplaceAll(r.FormValue("text"), "\r\n", "\n"))
	switch {
	case text == "":
		s.show(w, r, http.StatusBadRequest, "chat.error.empty", "")
		return
	case len([]rune(text)) > MaxText:
		s.show(w, r, http.StatusBadRequest, "chat.error.long", text)
		return
	}
	channel, _, err := s.channel(r)
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	content, err := event.New(event.MessageCreated, sess.Account, sess.Device, time.Now(),
		event.MessageCreatedData{Channel: channel, Text: text})
	if err == nil {
		_, err = s.scribe.Write(r.Context(), content)
	}
	if errors.Is(err, scribe.ErrStopped) {
		s.show(w, r, http.StatusServiceUnavailable, "error.stopped", text)
		return
	}
	if err != nil {
		log.Printf("chat: %v", err)
		s.show(w, r, http.StatusInternalServerError, "error.internal", text)
		return
	}
	http.Redirect(w, r, "/chat#end", http.StatusSeeOther)
}

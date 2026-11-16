// Package chat is Talk, the app of conversations: its home with the open
// channels of the community, and the conversation of each one. For now the
// community has one open channel, which every resident reads and writes.
package chat

import (
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/live"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/permissions"
	"github.com/earthler/earthler/core/scribe"
)

//go:embed chat.html home.html chat.js
var files embed.FS

// MaxText is the longest message, in characters.
const MaxText = 4000

// Area is the index this screen reads. A deleted message keeps its row,
// with no text and the id of the entry that deleted it, so the screen can
// say that something was there.
//
// The conversation is in the order of place, which is the time the message
// was written, and then of id: messages written on different machines take
// their place by time, and inside one journal the entry number decides. time
// is what the screen shows: the time of the message, or of its deletion.
var Area = index.Area{
	Name: "chat",
	Schema: `
		CREATE TABLE channels (number INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE authors (account INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE messages (
			number INTEGER PRIMARY KEY, channel INTEGER NOT NULL, account INTEGER NOT NULL,
			place INTEGER NOT NULL, time INTEGER NOT NULL, text TEXT NOT NULL, deleted INTEGER NOT NULL DEFAULT 0);
		CREATE INDEX messages_by_channel ON messages (channel, place, number);
		CREATE INDEX messages_by_deletion ON messages (deleted);`,
	Apply: apply,
}

func apply(tx *sql.Tx, entry index.Entry) error {
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
		_, err = tx.Exec(`INSERT OR IGNORE INTO channels (number, name) VALUES (?, ?)`, entry.ID, d.Name)
	case event.AccountCreated:
		var d event.AccountCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT OR IGNORE INTO authors (account, name) VALUES (?, ?)`, entry.ID, d.Name)
	case event.MessageCreated:
		var d event.MessageCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		// A deletion that arrived first, from another machine, has already
		// left the empty row, and it stays.
		_, err = tx.Exec(`INSERT OR IGNORE INTO messages (number, channel, account, place, time, text) VALUES (?, ?, ?, ?, ?, ?)`,
			entry.ID, d.Channel, e.Account, e.Time, e.Time, d.Text)
	case event.MessageDeleted:
		var d event.MessageDeletedData
		if e.Read(&d) != nil {
			return nil
		}
		return deleteMessage(tx, entry, e, d)
	}
	return err
}

// deleteMessage takes the text away. Once the entry that created the message
// is erased and the index is rebuilt, or when the deletion arrives before
// the message, there is no row to update, and the deletion creates the empty
// row itself. Every way must leave the same row: its place comes from the
// deletion, which carries the time of the message (an older deletion that
// does not carry it puts the row at the time of the deletion, both ways),
// and its time is the time of the deletion. Two deletions of the same
// message, written on two machines, keep the earlier one.
func deleteMessage(tx *sql.Tx, entry index.Entry, e event.Event, d event.MessageDeletedData) error {
	place, author := d.Placed, d.Author
	if place == 0 {
		place = e.Time
	}
	if author == 0 {
		author = e.Account
	}
	res, err := tx.Exec(`UPDATE messages SET text = '', deleted = ?1, time = ?2, place = ?4
		WHERE number = ?3 AND (deleted = 0 OR time > ?2 OR (time = ?2 AND deleted > ?1))`, entry.ID, e.Time, d.Message, place)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO messages (number, channel, account, place, time, text, deleted) VALUES (?, ?, ?, ?, ?, '', ?)`,
		d.Message, d.Channel, author, place, e.Time, entry.ID)
	return err
}

// Screen serves the conversation.
type Screen struct {
	index  *index.Index
	scribe *scribe.Scribe
	pages  *page.Renderer
	tmpl   *template.Template
	home   *template.Template
	hub    *live.Hub
}

// New returns the screen and starts listening to the scribe, so that every
// open page learns of a message as soon as it is written.
func New(ix *index.Index, s *scribe.Scribe, pages *page.Renderer) *Screen {
	sc := &Screen{index: ix, scribe: s, pages: pages, tmpl: pages.Screen(files, "chat.html"),
		home: pages.Screen(files, "home.html"), hub: live.NewHub()}
	s.Listen(sc.announce)
	return sc
}

// Routes adds the screen to the server. signedIn wraps what only residents
// may reach.
func (s *Screen) Routes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler) {
	mux.Handle("GET /talk/{$}", signedIn(http.HandlerFunc(s.showHome)))
	mux.Handle("GET /talk/channel/{channel}", signedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.show(w, r, http.StatusOK, "", "")
	})))
	mux.Handle("POST /talk/channel/{channel}", signedIn(http.HandlerFunc(s.post)))
	mux.Handle("GET /talk/live", signedIn(http.HandlerFunc(s.live)))
	mux.Handle("POST /talk/delete", signedIn(http.HandlerFunc(s.delete)))
	mux.HandleFunc("GET /talk/chat.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, files, "chat.js")
	})
	// The address of the conversation before there were four apps.
	mux.Handle("GET /chat", signedIn(http.HandlerFunc(s.toFirstChannel)))
}

// Paths is what the screen serves of Talk's bar.
var Paths = []string{"/talk/"}

// message is one card of the conversation.
type message struct {
	Number  uint64
	Account uint64
	Author  string
	Time    string
	Text    string
	Mine    bool
	Deleted uint64 // the entry that deleted it, or zero
	Place   int64  // where it goes in the conversation, with Number
	Channel uint64

	// Removable says that the reader may remove this message, though it is
	// not theirs, with a reason.
	Removable bool
}

type body struct {
	Channel  uint64
	Name     string // of the channel
	Position string // how far the page has seen, see live.Position
	Me       uint64 // the account of the person reading
	Messages []message
	Error    string // a text to show, or empty
	Draft    string // what the person had written when an error happened
	MaxText  int

	CanWrite  bool // the role allows writing in the channel
	CanRemove bool // the role allows removing what others wrote
}

// messageColumns is what every query of messages reads, in the order that
// scanMessage expects.
const messageColumns = `m.number, m.account, coalesce(a.name, ''), m.time, m.text, m.deleted, m.place, m.channel`

func (s *Screen) scanMessage(rows interface{ Scan(...any) error }, me uint64, canRemove bool) (message, error) {
	var m message
	var ms int64
	err := rows.Scan(&m.Number, &m.Account, &m.Author, &ms, &m.Text, &m.Deleted, &m.Place, &m.Channel)
	m.Time = time.UnixMilli(ms).Format(s.pages.T("time.short"))
	m.Mine = m.Account == me
	m.Removable = canRemove && !m.Mine
	return m, err
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request, status int, errKey, draft string) {
	sess := accounts.From(r.Context())
	b := body{Me: sess.Account, Draft: draft, MaxText: MaxText,
		CanWrite:  permissions.Can(sess.Role, permissions.WriteInOpenChannel),
		CanRemove: permissions.Can(sess.Role, permissions.RemoveContent)}
	if errKey != "" {
		b.Error = s.pages.T(errKey)
	}
	channel, name, err := s.channel(r)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	b.Channel, b.Name = channel, name
	// Read before the messages, so the page can only have seen more than it
	// says, never less: what it is sent twice, it shows once.
	b.Position = live.Position(s.index.Positions()).String()

	// The last messages, shown oldest first.
	rows, err := s.index.DB().QueryContext(r.Context(), `SELECT `+messageColumns+` FROM messages m
		LEFT JOIN authors a ON a.account = m.account
		WHERE m.channel = ? ORDER BY m.place DESC, m.number DESC LIMIT 200`, channel)
	if err != nil {
		log.Printf("chat: %v", err)
		http.Error(w, s.pages.T("error.internal"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		m, err := s.scanMessage(rows, sess.Account, b.CanRemove)
		if err != nil {
			log.Printf("chat: %v", err)
			continue
		}
		b.Messages = append(b.Messages, m)
	}
	for i, j := 0, len(b.Messages)-1; i < j; i, j = i+1, j-1 {
		b.Messages[i], b.Messages[j] = b.Messages[j], b.Messages[i]
	}
	s.pages.Render(w, status, s.tmpl, page.Data{Title: name, App: "talk", Tab: "conversations",
		Inner: &page.Inner{Title: name, Sub: s.pages.T("talk.channel.sub"), Back: "/talk/"},
		User:  sess.Name, Role: string(sess.Role), Script: "/talk/chat.js", Body: b})
}

// channel returns the open channel the address names.
func (s *Screen) channel(r *http.Request) (uint64, string, error) {
	number, err := strconv.ParseUint(r.PathValue("channel"), 10, 63)
	if err != nil {
		return 0, "", sql.ErrNoRows
	}
	var name string
	err = s.index.DB().QueryRowContext(r.Context(), `SELECT name FROM channels WHERE number = ?`, number).Scan(&name)
	return number, name, err
}

// toFirstChannel sends an old address of the conversation to the open
// channel the community started with.
func (s *Screen) toFirstChannel(w http.ResponseWriter, r *http.Request) {
	var number uint64
	if err := s.index.DB().QueryRowContext(r.Context(), `SELECT number FROM channels ORDER BY number LIMIT 1`).Scan(&number); err != nil {
		http.Redirect(w, r, "/talk/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/talk/channel/"+strconv.FormatUint(number, 10), http.StatusSeeOther)
}

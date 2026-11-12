package chat

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/live"
)

// Close ends the connections of the pages that are open and refuses new
// ones, so a server that is stopping does not wait for them.
func (s *Screen) Close() { s.hub.Close() }

// catchUpLimit is how many changes a page may have missed and still catch up
// on the same connection. Past that, it reloads instead.
const catchUpLimit = 500

// announce is called by the scribe after each batch reached the indexes. It
// turns the entries that change the conversation into messages for the
// open pages, rendered once for everyone.
func (s *Screen) announce(entries []journal.Entry) {
	for _, entry := range entries {
		e, err := event.Decode(entry.Content)
		if err != nil {
			continue
		}
		switch e.Type {
		case event.MessageCreated:
			m, err := s.message(context.Background(), entry.Number)
			if err != nil {
				log.Printf("chat: announcing entry %d: %v", entry.Number, err)
				continue
			}
			s.hub.Send(s.posted(m))
		case event.MessageDeleted:
			var d event.MessageDeletedData
			if e.Read(&d) == nil {
				s.hub.Send(live.Message{ID: entry.Number, Event: "deleted", Data: strconv.FormatUint(d.Message, 10)})
			}
		}
	}
}

// message reads one message from the index.
func (s *Screen) message(ctx context.Context, number uint64) (message, error) {
	row := s.index.DB().QueryRowContext(ctx, `SELECT `+messageColumns+` FROM messages m
		LEFT JOIN authors a ON a.account = m.account WHERE m.number = ?`, number)
	return s.scanMessage(row, 0, false)
}

// posted renders a message as the page would, with the controls of its
// author: the page keeps them only when the message is the reader's own.
func (s *Screen) posted(m message) live.Message {
	m.Mine = true
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "message", m); err != nil {
		log.Printf("chat: %v", err)
	}
	id := m.Number
	if m.Deleted > id {
		id = m.Deleted
	}
	return live.Message{ID: id, Event: "posted", Data: buf.String()}
}

// live is the connection every open page keeps. On arrival it sends what the
// page missed since the last entry it saw, then everything new.
func (s *Screen) live(w http.ResponseWriter, r *http.Request) {
	after := live.After(r)
	s.hub.Serve(w, r, func(send func(live.Message)) error {
		rows, err := s.index.DB().QueryContext(r.Context(), `SELECT `+messageColumns+` FROM messages m
			LEFT JOIN authors a ON a.account = m.account
			WHERE m.number > ?1 OR m.deleted > ?1 ORDER BY max(m.number, m.deleted) LIMIT ?2`, after, catchUpLimit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			if count++; count > catchUpLimit {
				send(live.Message{Event: "reload", Data: "too far behind"})
				return nil
			}
			m, err := s.scanMessage(rows, 0, false)
			if err != nil {
				return err
			}
			if m.Number <= after {
				// The page has the message; it only missed the deletion.
				send(live.Message{ID: m.Deleted, Event: "deleted", Data: strconv.FormatUint(m.Number, 10)})
				continue
			}
			send(s.posted(m))
		}
		return rows.Err()
	})
}

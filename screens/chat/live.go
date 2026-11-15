package chat

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/live"
)

// Close ends the connections of the pages that are open and refuses new
// ones, so a server that is stopping does not wait for them.
func (s *Screen) Close() { s.hub.Close() }

// catchUpLimit is how many changes a page may have missed and still catch up
// on the same connection. Past that, it reloads instead.
const catchUpLimit = 500

// announce is called by the scribe after each batch reached the indexes,
// whichever machine wrote it. It turns the entries that change the
// conversation into messages for the open pages, rendered once for everyone.
// Each message carries how far a page that received it has seen: the
// position of the index after the batch, with the journal of the batch at
// that very entry, so a page that loses the connection in the middle of a
// batch asks again for the rest.
func (s *Screen) announce(entries []index.Entry) {
	at := live.Position(s.index.Positions())
	for _, entry := range entries {
		e, err := event.Decode(entry.Content)
		if err != nil {
			continue
		}
		at[entry.Machine] = entry.Number
		switch e.Type {
		case event.MessageCreated:
			m, err := s.message(context.Background(), entry.ID)
			if err != nil {
				log.Printf("chat: announcing entry %d: %v", entry.ID, err)
				continue
			}
			s.hub.Send(s.posted(m, at.String()))
		case event.MessageDeleted:
			var d event.MessageDeletedData
			if e.Read(&d) == nil {
				s.hub.Send(live.Message{ID: at.String(), Event: "deleted", Data: strconv.FormatUint(d.Message, 10)})
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
func (s *Screen) posted(m message, id string) live.Message {
	m.Mine = true
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "message", m); err != nil {
		log.Printf("chat: %v", err)
	}
	return live.Message{ID: id, Event: "posted", Data: buf.String()}
}

// unseen is the condition, on a column that holds ids, that the page has not
// seen the entry: past its position in that machine's journal, or in the
// journal of a machine it knows nothing of.
func unseen(column string, after live.Position) (string, []any) {
	var parts []string
	var args []any
	var known []string
	for m, n := range after {
		parts = append(parts, "("+column+" / ? = ? AND "+column+" % ? > ?)")
		args = append(args, event.MachineSpan, m, event.MachineSpan, n)
		known = append(known, strconv.FormatUint(m, 10))
	}
	if len(known) == 0 {
		return "1 = 1", nil
	}
	parts = append(parts, "("+column+" / ? NOT IN ("+strings.Join(known, ",")+"))")
	args = append(args, event.MachineSpan)
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// live is the connection every open page keeps. On arrival it sends what the
// page missed since the position it says, in the order of the conversation,
// then the position of the index, then everything new.
func (s *Screen) live(w http.ResponseWriter, r *http.Request) {
	after := live.After(r)
	s.hub.Serve(w, r, func(send func(live.Message)) error {
		now := live.Position(s.index.Positions())
		// Only the journals this machine holds count: a page cannot have
		// seen anything of the others, and a position naming thousands of
		// machines must not make the query heavy.
		for m := range after {
			if _, held := now[m]; !held {
				delete(after, m)
			}
		}
		created, args := unseen("m.number", after)
		deleted, more := unseen("m.deleted", after)
		args = append(append(args, more...), catchUpLimit+1)
		rows, err := s.index.DB().QueryContext(r.Context(), `SELECT `+messageColumns+` FROM messages m
			LEFT JOIN authors a ON a.account = m.account
			WHERE `+created+` OR (m.deleted > 0 AND `+deleted+`) ORDER BY m.place, m.number LIMIT ?`, args...)
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
			// What is missed goes without a position, and the position comes
			// last: a page cut off in the middle asks again for everything.
			if after.Seen(m.Number) {
				// The page has the message; it only missed the deletion.
				send(live.Message{Event: "deleted", Data: strconv.FormatUint(m.Number, 10)})
				continue
			}
			send(s.posted(m, ""))
		}
		if err := rows.Err(); err != nil {
			return err
		}
		send(live.Message{ID: now.String(), Event: "position"})
		return nil
	})
}

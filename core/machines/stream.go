package machines

import (
	"bufio"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
)

// A machine serves only its own journal, and each machine follows each other
// one directly. A copy is never handed on: that way an entry of a machine's
// journal only ever comes from that machine.

// streamQueue is how many batches a slow stream may fall behind before it is
// dropped. The machine on the other side reconnects and catches up.
const streamQueue = 64

// streamBeat is an empty line that keeps a stream open through anything in
// the way and lets the other side notice a dead one.
var streamBeat = 20 * time.Second

type stream struct {
	ch      chan []journal.Entry
	dropped chan struct{}
	once    sync.Once
}

func (st *stream) drop() { st.once.Do(func() { close(st.dropped) }) }

// announce is told by the scribe about every batch; the batches of this
// machine's own journal go to every open stream.
func (s *Server) announce(entries []index.Entry) {
	if len(entries) == 0 || entries[0].Machine != s.Self {
		return
	}
	batch := make([]journal.Entry, len(entries))
	for i, e := range entries {
		batch[i] = e.Entry
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for st := range s.streams {
		select {
		case st.ch <- batch:
		default:
			st.drop()
			delete(s.streams, st)
		}
	}
}

func (s *Server) subscribe() (*stream, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, false
	}
	st := &stream{ch: make(chan []journal.Entry, streamQueue), dropped: make(chan struct{})}
	s.streams[st] = struct{}{}
	return st, true
}

func (s *Server) unsubscribe(st *stream) {
	s.mu.Lock()
	delete(s.streams, st)
	s.mu.Unlock()
}

// journal sends this machine's journal after the entry the other machine
// has, one line per entry exactly as it is in the file, and then every new
// entry as it is written. An empty line now and then says it is alive. With
// live=0 it ends once the other machine has everything written so far.
func (s *Server) journal(w http.ResponseWriter, r *http.Request) {
	after, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	if err != nil {
		http.Error(w, "after must be an entry number", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not possible", http.StatusInternalServerError)
		return
	}
	// Subscribed before reading, so nothing written meanwhile is missed.
	st, open := s.subscribe()
	if !open {
		http.Error(w, "the machine is stopping", http.StatusServiceUnavailable)
		return
	}
	defer s.unsubscribe(st)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Earthler-Machine", strconv.FormatUint(s.Self, 10))
	out := bufio.NewWriterSize(w, 64<<10)
	sent := after
	send := func(e journal.Entry) error {
		if e.Number <= sent {
			return nil
		}
		out.Write(journal.Line(e))
		out.WriteByte('\n')
		sent = e.Number
		return nil
	}
	if err := s.Own.Read(after+1, send); err != nil {
		return
	}
	if out.Flush() != nil {
		return
	}
	flusher.Flush()
	if r.URL.Query().Get("live") == "0" {
		return
	}

	beat := time.NewTicker(streamBeat)
	defer beat.Stop()
	for {
		select {
		case batch := <-st.ch:
			for _, e := range batch {
				if e.Number > sent+1 {
					return // a gap: the other side reconnects and reads it from the file
				}
				send(e)
			}
		case <-beat.C:
			out.WriteByte('\n')
		case <-st.dropped:
			return
		case <-r.Context().Done():
			return
		}
		if out.Flush() != nil {
			return
		}
		flusher.Flush()
	}
}

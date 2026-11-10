// Package live delivers what just happened to the pages that are open, over
// Server-Sent Events. A page keeps one connection, and the browser opens it
// again by itself after a dropped Wi-Fi, saying the number of the last entry
// it saw, so nothing is missed and nothing is shown twice.
package live

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message is one event sent to the pages.
type Message struct {
	ID    uint64 // the entry number the browser sends back when it reconnects; zero for none
	Event string // the name the page listens for
	Data  string // the payload; it may span lines
}

// queue is how many messages a slow connection may fall behind before it is
// dropped. Its browser then reconnects and catches up from the index.
const queue = 32

// heartbeat is a comment line that keeps the connection open through
// anything in the way and lets the browser notice a dead one.
var heartbeat = 25 * time.Second

// Hub is one group of connections that all receive the same messages.
type Hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

type subscriber struct {
	ch      chan Message
	dropped chan struct{}
	once    sync.Once
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[*subscriber]struct{}{}} }

// Send gives a message to every connection. It never waits: a connection
// whose queue is full is dropped instead, and its browser reconnects.
func (h *Hub) Send(m Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.ch <- m:
		default:
			s.once.Do(func() { close(s.dropped) })
			delete(h.subs, s)
		}
	}
}

// Connections says how many pages are listening.
func (h *Hub) Connections() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func (h *Hub) subscribe() *subscriber {
	s := &subscriber{ch: make(chan Message, queue), dropped: make(chan struct{})}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

func (h *Hub) unsubscribe(s *subscriber) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

// After reads the number of the last entry the browser saw: the one it sends
// on its own when it reconnects, or the one the page put in the address when
// it opened the connection.
func After(r *http.Request) uint64 {
	text := r.Header.Get("Last-Event-ID")
	if text == "" {
		text = r.URL.Query().Get("after")
	}
	n, _ := strconv.ParseUint(text, 10, 64)
	return n
}

// Serve streams messages to one page until the page leaves or falls too far
// behind. catchUp runs once, when the connection already receives new
// messages, so nothing written in between can be missed: it sends, through
// send, what the page missed since the entry it last saw.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, catchUp func(send func(Message)) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	sub := h.subscribe()
	defer h.unsubscribe(sub)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	var failed error
	write := func(m Message) {
		if failed == nil {
			_, failed = w.Write(encode(m))
		}
	}
	if _, err := w.Write([]byte("retry: 2000\n\n")); err != nil {
		return
	}
	if err := catchUp(write); err != nil || failed != nil {
		return
	}
	flusher.Flush()

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.dropped:
			return
		case m := <-sub.ch:
			write(m)
		case <-tick.C:
			_, failed = w.Write([]byte(": alive\n\n"))
		}
		if failed != nil {
			return
		}
		flusher.Flush()
	}
}

// encode writes a message in the wire format: one field per line and an
// empty line at the end. Data that spans lines becomes one data line per
// line, and the browser joins them again.
func encode(m Message) []byte {
	var b strings.Builder
	if m.ID > 0 {
		b.WriteString("id: ")
		b.WriteString(strconv.FormatUint(m.ID, 10))
		b.WriteByte('\n')
	}
	if m.Event != "" {
		b.WriteString("event: ")
		b.WriteString(m.Event)
		b.WriteByte('\n')
	}
	for _, line := range strings.Split(m.Data, "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return []byte(b.String())
}

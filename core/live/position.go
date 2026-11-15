package live

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/earthler/earthler/core/event"
)

// Position is how far a page has seen: the number of the last entry it saw
// in the journal of each machine. Machines write at the same time, so one
// number cannot say it. It travels as text, such as "0.120-1.33": entry 120
// of machine 0 and entry 33 of machine 1. A page carries it to whichever
// machine it reconnects to, and every machine reads it the same way.
type Position map[uint64]uint64

// String writes the position, by machine.
func (p Position) String() string {
	machines := make([]uint64, 0, len(p))
	for m := range p {
		machines = append(machines, m)
	}
	sort.Slice(machines, func(a, b int) bool { return machines[a] < machines[b] })
	parts := make([]string, 0, len(machines))
	for _, m := range machines {
		parts = append(parts, strconv.FormatUint(m, 10)+"."+strconv.FormatUint(p[m], 10))
	}
	return strings.Join(parts, "-")
}

// ParsePosition reads a position. A plain number, which is what a page
// opened before there were many machines sends, is a position in the journal
// of machine 0. What cannot be read is left out, which only means sending
// the page more than it needed.
func ParsePosition(text string) Position {
	p := Position{}
	if n, err := strconv.ParseUint(text, 10, 64); err == nil {
		p[0] = n
		return p
	}
	for _, part := range strings.Split(text, "-") {
		m, n, ok := strings.Cut(part, ".")
		machine, err1 := strconv.ParseUint(m, 10, 64)
		entry, err2 := strconv.ParseUint(n, 10, 64)
		if ok && err1 == nil && err2 == nil && machine <= event.MaxMachine && len(p) <= event.MaxMachine {
			p[machine] = entry
		}
	}
	return p
}

// Seen says whether the page has seen the entry with this id.
func (p Position) Seen(id uint64) bool {
	m, n := event.Split(id)
	return n <= p[m]
}

// With returns a copy of the position that has seen the entry with this id.
func (p Position) With(id uint64) Position {
	out := make(Position, len(p)+1)
	for m, n := range p {
		out[m] = n
	}
	m, n := event.Split(id)
	out[m] = max(out[m], n)
	return out
}

// After reads how far the browser has seen: what it sends on its own when it
// reconnects, or what the page put in the address when it opened the
// connection.
func After(r *http.Request) Position {
	text := r.Header.Get("Last-Event-ID")
	if text == "" {
		text = r.URL.Query().Get("after")
	}
	return ParsePosition(text)
}

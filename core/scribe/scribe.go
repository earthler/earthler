// Package scribe is the single writer of a machine. Every change goes
// through it: it writes the entries in the machine's own journal, applies
// them to the indexes and only then answers. Because the answer comes after
// the indexes, whoever wrote always sees what they wrote.
//
// A machine also keeps copies of the other machines' journals, and the
// scribe applies their entries to the same indexes. It never writes in them.
package scribe

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
)

// maxBatch is how many entries are written together at most.
const maxBatch = 50

// ErrStopped is returned when the scribe is not accepting writes: it was
// closed, or a write failed and the system is waiting for a restart.
var ErrStopped = errors.New("scribe: not accepting writes")

// Journals are the journals a machine holds.
type Journals struct {
	// Self is the number of this machine. The machine that founded the
	// community is 0.
	Self uint64

	// Own is the machine's own journal, which only this scribe writes.
	Own *journal.Journal

	// Kept are the copies of the other machines' journals, by machine.
	Kept map[uint64]*journal.Journal
}

// Scribe owns the journals and the indexes while it runs.
type Scribe struct {
	self    uint64
	journal *journal.Journal
	kept    map[uint64]*journal.Journal // only run touches it after Open
	indexes []*index.Index

	requests chan *request
	quit     chan struct{}
	stopped  chan struct{}
	once     sync.Once

	mu        sync.Mutex
	cause     error // why the scribe stopped by itself, if it did
	listeners []Listener

	// The clock rule. own is the latest time in this machine's journal, and
	// seen the latest time among the other machines' entries. Nothing is
	// written earlier than own, nor at or before seen. Only run touches them.
	own, seen int64
}

// Listener is told about every batch once the indexes applied it and before
// the writers get their answer, which is how a page that is open learns of
// a change before the page that made it. The scribe waits for it, so it
// must return at once and never write through the scribe itself.
type Listener func(entries []index.Entry)

// request is one thing for run to do: contents to write in the machine's
// own journal, entries another machine wrote, or a new journal to keep.
type request struct {
	contents [][]byte

	machine  uint64
	received []journal.Entry
	keep     *journal.Journal

	reply chan reply
}

func (r *request) write() bool { return r.contents != nil }

type reply struct {
	entries []index.Entry
	err     error
}

// Start starts the scribe of a machine that holds only its own journal, as
// the founding machine does while it is alone.
func Start(j *journal.Journal, indexes ...*index.Index) (*Scribe, error) {
	return Open(Journals{Own: j}, indexes...)
}

// Open brings every index level with every journal and starts the scribe.
func Open(js Journals, indexes ...*index.Index) (*Scribe, error) {
	if js.Own == nil || js.Self > event.MaxMachine {
		return nil, fmt.Errorf("scribe: machine %d needs its own journal and a number up to %d", js.Self, event.MaxMachine)
	}
	all := map[uint64]*journal.Journal{js.Self: js.Own}
	for m, j := range js.Kept {
		if m == js.Self || m > event.MaxMachine || j == nil {
			return nil, fmt.Errorf("scribe: the copy of machine %d cannot be kept by machine %d", m, js.Self)
		}
		all[m] = j
	}
	for _, ix := range indexes {
		if err := ix.Follow(all); err != nil {
			return nil, err
		}
	}
	kept := map[uint64]*journal.Journal{}
	for m, j := range js.Kept {
		kept[m] = j
	}
	s := &Scribe{
		self:     js.Self,
		journal:  js.Own,
		kept:     kept,
		indexes:  indexes,
		requests: make(chan *request),
		quit:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	var err error
	if s.own, err = latest(js.Own); err != nil {
		return nil, err
	}
	for _, j := range js.Kept {
		t, err := latest(j)
		if err != nil {
			return nil, err
		}
		s.seen = max(s.seen, t)
	}
	go s.run()
	return s, nil
}

// tail is how many entries at the end of a journal are read to find its
// latest time. An erased entry has lost its time, so one is not enough.
const tail = 64

// latest returns the latest time among the last entries of a journal.
func latest(j *journal.Journal) (int64, error) {
	last, _ := j.Last()
	if last == 0 {
		return 0, nil
	}
	from := uint64(1)
	if last > tail {
		from = last - tail + 1
	}
	var t int64
	err := j.Read(from, func(e journal.Entry) error {
		t = max(t, event.TimeOf(e.Content))
		return nil
	})
	return t, err
}

// Self returns the number of this machine.
func (s *Scribe) Self() uint64 { return s.self }

// Write records the contents, in order, as consecutive entries of the
// machine's own journal. It returns after the disk confirmed them and every
// index applied them.
func (s *Scribe) Write(ctx context.Context, contents ...[]byte) ([]index.Entry, error) {
	if len(contents) == 0 || len(contents) > maxBatch {
		return nil, fmt.Errorf("scribe: a write takes from 1 to %d contents", maxBatch)
	}
	// Checked here so one bad content only fails the request that sent it,
	// and not the others written in the same batch.
	for _, c := range contents {
		if err := journal.CheckContent(c); err != nil {
			return nil, err
		}
	}
	return s.ask(ctx, &request{contents: contents})
}

// Receive adds entries that machine wrote to the copy of its journal this
// machine keeps, and applies them to the indexes. Entries that do not follow
// the copy are refused with journal.ErrNotNext, and the scribe goes on.
func (s *Scribe) Receive(ctx context.Context, machine uint64, entries []journal.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	_, err := s.ask(ctx, &request{machine: machine, received: entries})
	return err
}

// Keep starts keeping the copy of another machine's journal, such as the
// journal of a machine that has just joined. What the copy already holds is
// applied to the indexes.
func (s *Scribe) Keep(ctx context.Context, machine uint64, j *journal.Journal) error {
	if j == nil {
		return fmt.Errorf("scribe: no journal to keep for machine %d", machine)
	}
	_, err := s.ask(ctx, &request{machine: machine, keep: j})
	return err
}

func (s *Scribe) ask(ctx context.Context, req *request) ([]index.Entry, error) {
	req.reply = make(chan reply, 1)
	select {
	case s.requests <- req:
	case <-s.stopped:
		return nil, s.stopError()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// Once the request was accepted the answer always comes, and it is
	// better to wait for it than to leave the caller not knowing.
	r := <-req.reply
	return r.entries, r.err
}

// Listen adds a listener.
func (s *Scribe) Listen(l Listener) {
	s.mu.Lock()
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()
}

// run is the only place that writes.
func (s *Scribe) run() {
	defer close(s.stopped)
	for {
		select {
		case <-s.quit:
			return
		case first := <-s.requests:
			if err := s.handle(first); err != nil {
				s.mu.Lock()
				s.cause = err
				s.mu.Unlock()
				return
			}
		}
	}
}

// handle does one request, and when it is a write, the writes already
// waiting with it. While the disk confirms one batch, the next requests pile
// up and form the next batch, so there is no need to wait on purpose. An
// error means the scribe has to stop.
func (s *Scribe) handle(first *request) error {
	if !first.write() {
		return s.other(first)
	}
	batch := []*request{first}
	count := len(first.contents)
	var after *request
collect:
	for count < maxBatch {
		select {
		case r := <-s.requests:
			if !r.write() {
				after = r
				break collect
			}
			batch = append(batch, r)
			count += len(r.contents)
		default:
			break collect
		}
	}
	if err := s.commit(batch); err != nil {
		if after != nil {
			after.reply <- reply{err: fmt.Errorf("%w: %v", ErrStopped, err)}
		}
		return err
	}
	if after != nil {
		return s.other(after)
	}
	return nil
}

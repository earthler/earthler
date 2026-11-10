// Package scribe is the single writer of the community. Every change goes
// through it: it writes the entries in the journal, applies them to the
// indexes and only then answers. Because the answer comes after the indexes,
// whoever wrote always sees what they wrote.
package scribe

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
)

// maxBatch is how many entries are written together at most.
const maxBatch = 50

// ErrStopped is returned when the scribe is not accepting writes: it was
// closed, or a write failed and the system is waiting for a restart.
var ErrStopped = errors.New("scribe: not accepting writes")

// Scribe owns the journal and the indexes while it runs.
type Scribe struct {
	journal *journal.Journal
	indexes []*index.Index

	requests chan *request
	quit     chan struct{}
	stopped  chan struct{}
	once     sync.Once

	mu        sync.Mutex
	cause     error // why the scribe stopped by itself, if it did
	listeners []Listener
}

// Listener is told about every batch once the indexes applied it and before
// the writers get their answer, which is how a page that is open learns of
// a change before the page that made it. The scribe waits for it, so it
// must return at once and never write through the scribe itself.
type Listener func(entries []journal.Entry)

type request struct {
	contents [][]byte
	reply    chan reply
}

type reply struct {
	entries []journal.Entry
	err     error
}

// Start brings every index level with the journal and starts the scribe.
func Start(j *journal.Journal, indexes ...*index.Index) (*Scribe, error) {
	for _, ix := range indexes {
		if err := ix.Follow(j); err != nil {
			return nil, err
		}
	}
	s := &Scribe{
		journal:  j,
		indexes:  indexes,
		requests: make(chan *request),
		quit:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go s.run()
	return s, nil
}

// Write records the contents, in order, as consecutive entries. It returns
// after the disk confirmed them and every index applied them.
func (s *Scribe) Write(ctx context.Context, contents ...[]byte) ([]journal.Entry, error) {
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
	req := &request{contents: contents, reply: make(chan reply, 1)}
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
			// Take what is already waiting. While the disk confirms one
			// batch, the next requests pile up and form the next batch, so
			// there is no need to wait on purpose.
			batch := []*request{first}
			count := len(first.contents)
		collect:
			for count < maxBatch {
				select {
				case r := <-s.requests:
					batch = append(batch, r)
					count += len(r.contents)
				default:
					break collect
				}
			}
			if err := s.commit(batch); err != nil {
				s.mu.Lock()
				s.cause = err
				s.mu.Unlock()
				return
			}
		}
	}
}

// commit writes one batch. An error means the scribe has to stop: either the
// journal refused the write, or the entries are in the journal but an index
// did not take them, and only a restart brings the indexes level again.
func (s *Scribe) commit(batch []*request) error {
	var contents [][]byte
	for _, r := range batch {
		contents = append(contents, r.contents...)
	}
	entries, err := s.journal.Append(contents...)
	if err == nil {
		for _, ix := range s.indexes {
			if err = ix.Apply(entries); err != nil {
				break
			}
		}
	}
	if err == nil {
		s.mu.Lock()
		listeners := s.listeners
		s.mu.Unlock()
		for _, l := range listeners {
			l(entries)
		}
	}
	next := 0
	for _, r := range batch {
		if err != nil {
			r.reply <- reply{err: fmt.Errorf("%w: %v", ErrStopped, err)}
			continue
		}
		r.reply <- reply{entries: entries[next : next+len(r.contents) : next+len(r.contents)]}
		next += len(r.contents)
	}
	return err
}

func (s *Scribe) stopError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cause != nil {
		return fmt.Errorf("%w: %v", ErrStopped, s.cause)
	}
	return ErrStopped
}

// Err reports why the scribe stopped by itself, or nil while it is running
// or after a normal Close.
func (s *Scribe) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cause
}

// Close stops the scribe after the batch it is writing. It does not close
// the journal or the indexes.
func (s *Scribe) Close() {
	s.once.Do(func() { close(s.quit) })
	<-s.stopped
}

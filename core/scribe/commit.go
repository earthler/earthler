package scribe

import (
	"fmt"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
)

// The earliest time a new entry may carry is not before the latest entry of
// this machine, and after every entry it has seen from the others. Inside
// one journal the entry number orders entries with the same time.

// commit writes one batch. An error means the scribe has to stop: either the
// journal refused the write, or the entries are in the journal but an index
// did not take them, and only a restart brings the indexes level again.
func (s *Scribe) commit(batch []*request) error {
	// The floor rises with each content: two requests in one batch were
	// stamped by their screens in any order, and the second must not carry
	// a time earlier than the first.
	var contents [][]byte
	own := s.own
	for _, r := range batch {
		for _, c := range r.contents {
			c, t, err := event.Stamp(c, max(own, s.seen+1))
			if err != nil {
				return s.refuse(batch, err)
			}
			own = max(own, t)
			contents = append(contents, c)
		}
	}
	written, err := s.journal.Append(contents...)
	if err != nil {
		return s.refuse(batch, err)
	}
	s.own = own
	for _, ix := range s.indexes {
		if err := ix.Apply(s.self, written); err != nil {
			return s.refuse(batch, err)
		}
	}
	entries := index.Wrap(s.self, written)
	s.mu.Lock()
	listeners := s.listeners
	s.mu.Unlock()
	for _, l := range listeners {
		l(entries)
	}
	next := 0
	for _, r := range batch {
		r.reply <- reply{entries: entries[next : next+len(r.contents) : next+len(r.contents)]}
		next += len(r.contents)
	}
	return nil
}

// refuse answers every writer of the batch with the error that stops the
// scribe, and returns it.
func (s *Scribe) refuse(batch []*request, err error) error {
	for _, r := range batch {
		r.reply <- reply{err: fmt.Errorf("%w: %v", ErrStopped, err)}
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
// the journals or the indexes.
func (s *Scribe) Close() {
	s.once.Do(func() { close(s.quit) })
	<-s.stopped
}

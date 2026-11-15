package scribe

import (
	"fmt"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
)

// other does a request that is not a write: entries another machine wrote,
// or a new journal to keep. A copy that refuses entries is a matter of the
// other machine, and the scribe goes on. An index that fails after the copy
// took the entries is not, and the scribe stops, as it does for a write.
func (s *Scribe) other(r *request) error {
	if r.keep != nil {
		err, stop := s.keep(r)
		r.reply <- reply{err: err}
		return stop
	}
	j := s.kept[r.machine]
	if j == nil {
		r.reply <- reply{err: fmt.Errorf("scribe: machine %d keeps no copy of machine %d", s.self, r.machine)}
		return nil
	}
	if err := j.Receive(r.received); err != nil {
		r.reply <- reply{err: err}
		return nil
	}
	for _, ix := range s.indexes {
		if err := ix.Apply(r.machine, r.received); err != nil {
			r.reply <- reply{err: fmt.Errorf("%w: %v", ErrStopped, err)}
			return err
		}
	}
	for _, e := range r.received {
		s.seen = max(s.seen, event.TimeOf(e.Content))
	}
	s.tell(index.Wrap(r.machine, r.received))
	r.reply <- reply{}
	return nil
}

// keep adds a journal to keep. It returns the error for the one who asked,
// and the error that stops the scribe, if any.
func (s *Scribe) keep(r *request) (err, stop error) {
	if r.machine == s.self || r.machine > event.MaxMachine || s.kept[r.machine] != nil {
		return fmt.Errorf("scribe: machine %d cannot keep a copy of machine %d", s.self, r.machine), nil
	}
	for _, ix := range s.indexes {
		if err := ix.FollowOne(r.machine, r.keep); err != nil {
			return fmt.Errorf("%w: %v", ErrStopped, err), err
		}
	}
	t, err := latest(r.keep)
	if err != nil {
		return err, nil
	}
	s.seen = max(s.seen, t)
	s.kept[r.machine] = r.keep
	return nil, nil
}

// tell gives a batch to the listeners.
func (s *Scribe) tell(entries []index.Entry) {
	s.mu.Lock()
	listeners := s.listeners
	s.mu.Unlock()
	for _, l := range listeners {
		l(entries)
	}
}

package journal

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

// ErrNotNext is returned by Receive when an entry does not follow the last
// one held: another number, or a seal of a history that is not this one.
var ErrNotNext = errors.New("journal: the entry does not follow the last one held")

// Line returns an entry as it is written in a journal file, without the
// final newline. It is how an entry travels from one machine to another.
func Line(e Entry) []byte {
	b := appendLine(nil, e)
	return b[:len(b)-1]
}

// ParseLine reads one line of a journal, without its final newline. It
// checks the shape of the line and that the content matches its digest; it
// does not check the line against the ones before it, which Receive does.
func ParseLine(line []byte) (Entry, error) { return parseLine(line) }

// Receive adds entries that another machine wrote, exactly as they are in
// that machine's journal: the same numbers, the same seals. It is how a
// machine keeps its copy of another machine's journal, which only grows the
// way the original does.
//
// Each entry must follow the one before, in number and in seal, and its
// content must match its digest, unless it arrives already erased. An entry
// that does not follow is refused with ErrNotNext and nothing is written; the
// journal can still receive. Receive returns only after the disk confirmed
// the write.
func (j *Journal) Receive(entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return ErrClosed
	}
	if j.failed != nil {
		return fmt.Errorf("%w (cause: %v)", ErrFailed, j.failed)
	}

	number, seal := j.last, j.lastSeal
	var buf []byte
	for _, e := range entries {
		switch {
		case e.Number != number+1:
			return fmt.Errorf("%w: entry %d came after entry %d", ErrNotNext, e.Number, number)
		case e.PrevSeal != seal:
			return fmt.Errorf("%w: entry %d is sealed to another history", ErrNotNext, e.Number)
		case e.Erased && e.Content != nil:
			return fmt.Errorf("journal: entry %d is erased and has a content", e.Number)
		case !e.Erased && sha256.Sum256(e.Content) != e.Digest:
			return fmt.Errorf("journal: entry %d does not match its digest", e.Number)
		}
		if !e.Erased {
			if err := CheckContent(e.Content); err != nil {
				return fmt.Errorf("journal: entry %d: %w", e.Number, err)
			}
		}
		buf = appendLine(buf, e)
		number, seal = e.Number, e.Seal()
	}

	if j.file == nil || j.size >= j.segmentSize {
		if err := j.startSegment(); err != nil {
			return j.fail(err)
		}
	}
	if _, err := j.file.Write(buf); err != nil {
		return j.fail(err)
	}
	if err := j.sync(j.file); err != nil {
		return j.fail(err)
	}
	j.last, j.lastSeal = number, seal
	j.size += int64(len(buf))
	return nil
}

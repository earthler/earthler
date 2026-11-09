package journal

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// DamageError says where the journal stopped being trustworthy.
type DamageError struct {
	Segment string // name of the file
	Entry   uint64 // number of the first entry that cannot be trusted
	Offset  int64  // where that entry starts in the file, in bytes
	Reason  string
}

func (e *DamageError) Error() string {
	return fmt.Sprintf("journal: damage in %s at entry %d (byte %d): %s", e.Segment, e.Entry, e.Offset, e.Reason)
}

// errStop is returned by a callback to end a scan early without an error.
var errStop = errors.New("stop")

type scanResult struct {
	count      uint64 // complete, valid entries read
	lastNumber uint64
	lastSeal   Seal
	end        int64 // offset just past the last valid line
	torn       int64 // bytes of an unfinished line after end
}

// scanSegment reads a segment from the start and checks every line: its
// shape, its digest, its number and its tie to the line before. first is the
// number the first entry must have. prev is the seal the first entry must
// point to, or nil when that is not known. fn, when not nil, receives each
// valid entry and may return errStop.
//
// An unfinished line at the end is reported in torn and is not an error. A
// complete line that fails a check is a *DamageError.
func scanSegment(path string, first uint64, prev *Seal, fn func(Entry) error) (scanResult, error) {
	var res scanResult
	f, err := os.Open(path)
	if err != nil {
		return res, err
	}
	defer f.Close()

	damage := func(reason string) error {
		return &DamageError{Segment: filepath.Base(path), Entry: first + res.count, Offset: res.end, Reason: reason}
	}

	var lineSize int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), MaxContent+512)
	sc.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			lineSize = int64(i + 1)
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			res.torn = int64(len(data))
			return len(data), nil, bufio.ErrFinalToken
		}
		return 0, nil, nil
	})

	for sc.Scan() {
		if res.torn > 0 {
			break // the unfinished line is not an entry
		}
		e, err := parseLine(sc.Bytes())
		if err != nil {
			return res, damage(err.Error())
		}
		if e.Number != first+res.count {
			return res, damage(fmt.Sprintf("found number %d", e.Number))
		}
		if prev != nil && e.PrevSeal != *prev {
			return res, damage("the entry is not sealed to the one before it")
		}
		seal := e.Seal()
		prev = &seal
		res.count++
		res.lastNumber, res.lastSeal = e.Number, seal
		res.end += lineSize
		if fn != nil {
			if err := fn(e); err != nil {
				if errors.Is(err, errStop) {
					return res, nil
				}
				return res, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		if !errors.Is(err, bufio.ErrTooLong) {
			return res, err
		}
		// A very long run of bytes with no newline up to the end of the file
		// is still an unfinished line: a large write that was cut short.
		rest, err := unfinishedTail(f, res.end)
		if err != nil {
			return res, err
		}
		if rest == 0 {
			return res, damage("the line is longer than any entry can be")
		}
		res.torn = rest
	}
	return res, nil
}

// unfinishedTail returns how many bytes follow offset when none of them is a
// newline, and zero when there is a newline after offset.
func unfinishedTail(f *os.File, offset int64) (int64, error) {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	var total int64
	buf := make([]byte, 64<<10)
	for {
		n, err := f.Read(buf)
		if bytes.IndexByte(buf[:n], '\n') >= 0 {
			return 0, nil
		}
		total += int64(n)
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return 0, err
		}
	}
}

// Read calls fn for every entry from number from to the last entry that was
// confirmed when Read was called, in order. It checks each segment it reads
// from its first line, so a damaged entry is reported, never delivered.
func (j *Journal) Read(from uint64, fn func(Entry) error) error {
	j.mu.Lock()
	last := j.last
	segments := slices.Clone(j.segments)
	j.mu.Unlock()

	if from == 0 {
		from = 1
	}
	if from > last {
		return nil
	}
	// The segment that holds from is the last one that starts at or before it.
	i := sort.Search(len(segments), func(i int) bool { return segments[i] > from }) - 1
	if i < 0 {
		return fmt.Errorf("journal: the segment that holds entry %d is missing", from)
	}
	for ; i < len(segments); i++ {
		finished := false
		_, err := scanSegment(j.segmentPath(segments[i]), segments[i], nil, func(e Entry) error {
			if e.Number >= from {
				if err := fn(e); err != nil {
					return err
				}
			}
			if e.Number == last {
				finished = true
				return errStop
			}
			return nil
		})
		if err != nil || finished {
			return err
		}
	}
	return fmt.Errorf("journal: entry %d was confirmed but is not in the files", last)
}

// Verify reads the whole journal and checks every entry and every seal, from
// entry 1 to the last entry that was confirmed when Verify was called. It
// returns a *DamageError that names the first entry that fails.
func (j *Journal) Verify() error {
	j.mu.Lock()
	last, lastSeal := j.last, j.lastSeal
	segments := slices.Clone(j.segments)
	j.mu.Unlock()

	if last == 0 {
		return nil
	}
	if len(segments) == 0 || segments[0] != 1 {
		return &DamageError{Segment: "(none)", Entry: 1, Reason: "the first segment is missing"}
	}
	prev := new(Seal) // entry 1 points to the zero seal
	next := uint64(1)
	for _, first := range segments {
		name := filepath.Base(j.segmentPath(first))
		if first != next {
			return &DamageError{Segment: name, Entry: next, Reason: "entries are missing before this segment"}
		}
		finished := false
		res, err := scanSegment(j.segmentPath(first), first, prev, func(e Entry) error {
			if e.Number == last {
				finished = true
				return errStop
			}
			return nil
		})
		if err != nil {
			return err
		}
		if finished {
			if res.lastSeal != lastSeal {
				return &DamageError{Segment: name, Entry: last, Offset: res.end, Reason: "the last seal changed since the journal was opened"}
			}
			return nil
		}
		if res.count == 0 {
			return &DamageError{Segment: name, Entry: first, Reason: "the segment is empty"}
		}
		if res.torn > 0 {
			return &DamageError{Segment: name, Entry: res.lastNumber + 1, Offset: res.end, Reason: "unfinished line before the end of the journal"}
		}
		seal := res.lastSeal
		prev, next = &seal, res.lastNumber+1
	}
	return &DamageError{Segment: "(none)", Entry: next, Reason: "the journal ends before its last confirmed entry"}
}

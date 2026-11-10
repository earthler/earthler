package journal

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
)

// DefaultSegmentSize is the size at which a new segment file is started.
const DefaultSegmentSize = 64 << 20

const segmentSuffix = ".journal"

var segmentName = regexp.MustCompile(`^[0-9]{20}\.journal$`)

var (
	// ErrFailed is returned after a write could not be confirmed. The journal
	// accepts nothing more until it is opened again, because after a failed
	// confirmation nobody knows what is on the disk.
	ErrFailed = errors.New("journal: a write failed; open the journal again to continue")

	// ErrClosed is returned by a journal that was closed.
	ErrClosed = errors.New("journal: closed")
)

// Options change how a journal is opened. The zero value is ready to use.
type Options struct {
	// SegmentSize is the size, in bytes, at which a new segment file is
	// started. Zero means DefaultSegmentSize.
	SegmentSize int64
}

// Journal is an open journal. Append is meant for a single writer. Read and
// Verify can run at the same time as Append.
type Journal struct {
	dir         string
	segmentSize int64

	mu       sync.Mutex
	lock     *os.File
	segments []uint64 // number of the first entry of each segment, ascending
	file     *os.File // the last segment, open for appending
	size     int64    // confirmed size of the last segment
	last     uint64   // number of the last confirmed entry
	lastSeal Seal     // seal of the last confirmed entry
	failed   error
	closed   bool

	sync func(*os.File) error // os.File.Sync, replaced in tests
}

// Open opens the journal kept in dir, creating the folder if needed.
//
// It reads the last segment and cuts an unfinished line at its end, which is
// what a power cut in the middle of a write leaves behind. If a complete
// line is damaged, Open returns a *DamageError and changes nothing.
func Open(dir string, opts Options) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock, err := lockDir(dir)
	if err != nil {
		return nil, err
	}
	j := &Journal{dir: dir, segmentSize: opts.SegmentSize, lock: lock, sync: (*os.File).Sync}
	if j.segmentSize <= 0 {
		j.segmentSize = DefaultSegmentSize
	}
	if err := j.recover(); err != nil {
		lock.Close()
		return nil, err
	}
	return j, nil
}

// recover finds the last confirmed entry and leaves the last segment ready
// for appending.
func (j *Journal) recover() error {
	if err := removeRewrites(j.dir); err != nil {
		return err
	}
	names, err := os.ReadDir(j.dir)
	if err != nil {
		return err
	}
	for _, n := range names {
		if segmentName.MatchString(n.Name()) {
			first, _ := strconv.ParseUint(n.Name()[:20], 10, 64)
			j.segments = append(j.segments, first)
		}
	}
	sort.Slice(j.segments, func(a, b int) bool { return j.segments[a] < j.segments[b] })

	// Walk back from the last segment. A segment can be empty when the
	// power went out right after it was created.
	for len(j.segments) > 0 {
		first := j.segments[len(j.segments)-1]
		path := j.segmentPath(first)
		res, err := scanSegment(path, first, nil, nil)
		if err != nil {
			return err
		}
		if res.torn > 0 {
			if err := truncate(path, res.end); err != nil {
				return err
			}
		}
		if res.count == 0 {
			if err := os.Remove(path); err != nil {
				return err
			}
			if err := syncDir(j.dir); err != nil {
				return err
			}
			j.segments = j.segments[:len(j.segments)-1]
			continue
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		j.file, j.size = f, res.end
		j.last, j.lastSeal = res.lastNumber, res.lastSeal
		return nil
	}
	return nil
}

// Append adds one entry for each content and returns the entries, in the
// same order. It returns only after the disk confirmed the write, so an
// entry that Append reported is never lost. The contents of one call are
// written together.
func (j *Journal) Append(contents ...[]byte) ([]Entry, error) {
	if len(contents) == 0 {
		return nil, errors.New("journal: nothing to append")
	}
	size := 0
	for _, c := range contents {
		if err := CheckContent(c); err != nil {
			return nil, err
		}
		size += len(c) + 160
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, ErrClosed
	}
	if j.failed != nil {
		return nil, fmt.Errorf("%w (cause: %v)", ErrFailed, j.failed)
	}
	if j.file == nil || j.size >= j.segmentSize {
		if err := j.startSegment(); err != nil {
			return nil, j.fail(err)
		}
	}

	buf := make([]byte, 0, size)
	entries := make([]Entry, len(contents))
	number, seal := j.last, j.lastSeal
	for i, c := range contents {
		number++
		entries[i] = Entry{Number: number, PrevSeal: seal, Digest: sha256.Sum256(c), Content: c}
		buf = appendLine(buf, entries[i])
		seal = entries[i].Seal()
	}
	if _, err := j.file.Write(buf); err != nil {
		return nil, j.fail(err)
	}
	if err := j.sync(j.file); err != nil {
		return nil, j.fail(err)
	}
	j.last, j.lastSeal = number, seal
	j.size += int64(len(buf))
	return entries, nil
}

// fail puts the journal in the failed state. It tries to take back what was
// written, but does not rely on that: the next Open reads the file again.
func (j *Journal) fail(err error) error {
	j.failed = err
	if j.file != nil {
		j.file.Truncate(j.size)
	}
	return fmt.Errorf("%w (cause: %v)", ErrFailed, err)
}

// startSegment creates the file for the next entry and confirms both the
// file and the folder, so the new name survives a power cut.
func (j *Journal) startSegment() error {
	first := j.last + 1
	f, err := os.OpenFile(j.segmentPath(first), os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := j.sync(f); err != nil {
		f.Close()
		return err
	}
	if err := syncDir(j.dir); err != nil {
		f.Close()
		return err
	}
	if j.file != nil {
		j.file.Close()
	}
	j.file, j.size = f, 0
	j.segments = append(j.segments, first)
	return nil
}

// Last returns the number and the seal of the last confirmed entry. An empty
// journal returns zero and a zero seal.
func (j *Journal) Last() (uint64, Seal) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.last, j.lastSeal
}

// Close releases the journal. Entries that Append reported are already on
// the disk, so Close has nothing to save.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	var err error
	if j.file != nil {
		err = j.file.Close()
	}
	if cerr := j.lock.Close(); err == nil {
		err = cerr
	}
	return err
}

func (j *Journal) segmentPath(first uint64) string {
	return filepath.Join(j.dir, fmt.Sprintf("%020d%s", first, segmentSuffix))
}

// truncate cuts a file at size and confirms the cut.
func truncate(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// syncDir confirms a folder, which is what makes a new or removed file name
// survive a power cut.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}

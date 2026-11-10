package journal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

// rewriteSuffix marks a segment being rewritten. One left behind by a power
// cut is removed the next time the journal is opened.
const rewriteSuffix = ".rewrite"

// Erase replaces the content of the given entries with the erased mark. Each
// entry keeps its number, its digest and its seals, so the chain stays valid
// and Verify still passes. Every segment that holds one of the entries is
// rewritten into a new file, confirmed, and moved over the old one, which
// the file system does in one step. An entry that is already erased is left
// as it is, so Erase can be repeated after a failure.
func (j *Journal) Erase(numbers ...uint64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return ErrClosed
	}
	if j.failed != nil {
		return fmt.Errorf("%w (cause: %v)", ErrFailed, j.failed)
	}
	targets := map[uint64]bool{}
	for _, n := range numbers {
		if n == 0 || n > j.last {
			return fmt.Errorf("journal: entry %d does not exist", n)
		}
		targets[n] = true
	}
	for i, first := range j.segments {
		last := j.last
		if i+1 < len(j.segments) {
			last = j.segments[i+1] - 1
		}
		if !holdsAny(targets, first, last) {
			continue
		}
		if err := j.rewrite(i, targets); err != nil {
			return err
		}
	}
	return nil
}

func holdsAny(targets map[uint64]bool, first, last uint64) bool {
	for n := range targets {
		if n >= first && n <= last {
			return true
		}
	}
	return false
}

// rewrite writes segment i again without the contents of the targets.
func (j *Journal) rewrite(i int, targets map[uint64]bool) error {
	first := j.segments[i]
	path := j.segmentPath(first)
	var buf bytes.Buffer
	changed := false
	res, err := scanSegment(path, first, nil, func(e Entry) error {
		if targets[e.Number] && !e.Erased {
			e.Erased, e.Content = true, nil
			changed = true
		}
		buf.Write(appendLine(nil, e))
		return nil
	})
	if err != nil {
		return err
	}
	if res.torn > 0 {
		return &DamageError{Segment: path, Entry: res.lastNumber + 1, Offset: res.end, Reason: "unfinished line in a segment being rewritten"}
	}
	if !changed {
		return nil
	}

	tmp := path + rewriteSuffix
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := j.sync(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if err := syncDir(j.dir); err != nil {
		return err
	}
	if i == len(j.segments)-1 && j.file != nil {
		// The writer was appending to the old file. From now on it appends
		// to the new one.
		j.file.Close()
		if j.file, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0); err != nil {
			return j.fail(err)
		}
		j.size = int64(buf.Len())
	}
	return nil
}

// removeRewrites deletes the files that an interrupted Erase left behind.
func removeRewrites(dir string) error {
	names, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, n := range names {
		if len(n.Name()) > len(rewriteSuffix) && n.Name()[len(n.Name())-len(rewriteSuffix):] == rewriteSuffix {
			if err := os.Remove(dir + string(os.PathSeparator) + n.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

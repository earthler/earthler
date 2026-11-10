package journal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// open opens a journal for a test. Confirming every write with the disk is
// what the real journal does, but it makes tests with many entries slow, so
// most tests skip it. The tests about confirmation set j.sync themselves.
func open(t *testing.T, dir string, opts Options) *Journal {
	t.Helper()
	j, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	j.sync = func(*os.File) error { return nil }
	t.Cleanup(func() { j.Close() })
	return j
}

func content(i int) []byte {
	return []byte(fmt.Sprintf(`{"n":%d,"text":"entry %d"}`, i, i))
}

func appendN(t *testing.T, j *Journal, from, n int) {
	t.Helper()
	for i := from; i < from+n; i++ {
		if _, err := j.Append(content(i)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
}

func readAll(t *testing.T, j *Journal, from uint64) []Entry {
	t.Helper()
	var entries []Entry
	if err := j.Read(from, func(e Entry) error { entries = append(entries, e); return nil }); err != nil {
		t.Fatalf("read: %v", err)
	}
	return entries
}

func segmentFiles(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*"+segmentSuffix))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestAppendAndRead(t *testing.T) {
	j := open(t, t.TempDir(), Options{})
	if n, seal := j.Last(); n != 0 || seal != (Seal{}) {
		t.Fatalf("an empty journal reports entry %d", n)
	}
	added, err := j.Append(content(1), content(2), content(3))
	if err != nil || len(added) != 3 || added[0].Number != 1 || added[2].Number != 3 {
		t.Fatalf("added = %v, err = %v", added, err)
	}
	added, err = j.Append(content(4))
	if err != nil || len(added) != 1 || added[0].Number != 4 || added[0].PrevSeal == (Seal{}) {
		t.Fatalf("added = %v, err = %v", added, err)
	}

	entries := readAll(t, j, 1)
	if len(entries) != 4 {
		t.Fatalf("read %d entries, want 4", len(entries))
	}
	var prev Seal
	for i, e := range entries {
		if e.Number != uint64(i+1) || !bytes.Equal(e.Content, content(i+1)) {
			t.Errorf("entry %d: number %d, content %s", i+1, e.Number, e.Content)
		}
		if e.PrevSeal != prev {
			t.Errorf("entry %d is not sealed to the one before it", e.Number)
		}
		prev = e.Seal()
	}
	if n, seal := j.Last(); n != 4 || seal != prev {
		t.Errorf("Last reports entry %d or the wrong seal", n)
	}
	if got := readAll(t, j, 3); len(got) != 2 || got[0].Number != 3 {
		t.Errorf("reading from 3 returned %d entries", len(got))
	}
	if got := readAll(t, j, 5); len(got) != 0 {
		t.Errorf("reading past the end returned %d entries", len(got))
	}
	if err := j.Verify(); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestReopenContinuesTheChain(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{})
	appendN(t, j, 1, 5)
	_, sealBefore := j.Last()
	j.Close()

	j = open(t, dir, Options{})
	if n, seal := j.Last(); n != 5 || seal != sealBefore {
		t.Fatalf("after reopening, Last reports entry %d or another seal", n)
	}
	appendN(t, j, 6, 5)
	if err := j.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got := readAll(t, j, 1); len(got) != 10 {
		t.Fatalf("read %d entries, want 10", len(got))
	}
}

func TestSegmentsRotate(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{SegmentSize: 1024})
	appendN(t, j, 1, 100)
	files := segmentFiles(t, dir)
	if len(files) < 5 {
		t.Fatalf("only %d segments for 100 entries with a 1 KB limit", len(files))
	}
	if filepath.Base(files[0]) != "00000000000000000001.journal" {
		t.Errorf("first segment is named %s", filepath.Base(files[0]))
	}
	if got := readAll(t, j, 1); len(got) != 100 {
		t.Fatalf("read %d entries across segments, want 100", len(got))
	}
	if got := readAll(t, j, 57); len(got) != 44 || got[0].Number != 57 {
		t.Fatalf("reading from 57 returned %d entries", len(got))
	}
	if err := j.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	j.Close()

	j = open(t, dir, Options{SegmentSize: 1024})
	appendN(t, j, 101, 20)
	if err := j.Verify(); err != nil {
		t.Fatalf("verify after reopening: %v", err)
	}
	if n, _ := j.Last(); n != 120 {
		t.Fatalf("last entry is %d, want 120", n)
	}
}

func TestSecondOpenIsRefused(t *testing.T) {
	dir := t.TempDir()
	open(t, dir, Options{})
	if _, err := Open(dir, Options{}); err == nil {
		t.Fatal("the same journal was opened twice")
	}
}

func TestBadContentIsRefused(t *testing.T) {
	j := open(t, t.TempDir(), Options{})
	bad := map[string][]byte{
		"empty":       nil,
		"not json":    []byte(`{"a":`),
		"not object":  []byte(`[1,2]`),
		"two lines":   []byte("{\"a\":1,\n\"b\":2}"),
		"too large":   []byte(`{"a":"` + strings.Repeat("x", MaxContent) + `"}`),
		"plain value": []byte(`"text"`),
	}
	for name, c := range bad {
		if _, err := j.Append(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := j.Append(); err == nil {
		t.Error("an empty call was accepted")
	}
	if _, err := j.Append(content(1), []byte(`nope`)); err == nil {
		t.Error("a batch with one bad content was accepted")
	}
	if n, _ := j.Last(); n != 0 {
		t.Fatalf("refused contents left %d entries behind", n)
	}
}

func TestFailedConfirmationStopsTheJournal(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{})
	appendN(t, j, 1, 3)

	j.sync = func(*os.File) error { return errors.New("disk said no") }
	if _, err := j.Append(content(4)); !errors.Is(err, ErrFailed) {
		t.Fatalf("want ErrFailed, got %v", err)
	}
	j.sync = func(*os.File) error { return nil }
	if _, err := j.Append(content(4)); !errors.Is(err, ErrFailed) {
		t.Fatalf("the journal accepted a write after a failed confirmation: %v", err)
	}
	if n, _ := j.Last(); n != 3 {
		t.Fatalf("last entry is %d, want 3", n)
	}
	j.Close()

	j = open(t, dir, Options{})
	if n, _ := j.Last(); n != 3 {
		t.Fatalf("after reopening, last entry is %d, want 3", n)
	}
	appendN(t, j, 4, 1)
	if err := j.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestClosedJournalRefusesWrites(t *testing.T) {
	j := open(t, t.TempDir(), Options{})
	j.Close()
	if _, err := j.Append(content(1)); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

func TestReadWhileAppending(t *testing.T) {
	j := open(t, t.TempDir(), Options{SegmentSize: 2048})
	const total = 400
	done := make(chan error, 1)
	go func() {
		for i := 1; i <= total; i++ {
			if _, err := j.Append(content(i)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for running := true; running; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			running = false
		default:
		}
		before, _ := j.Last()
		next := uint64(1)
		err := j.Read(1, func(e Entry) error {
			if e.Number != next {
				return fmt.Errorf("read entry %d, want %d", e.Number, next)
			}
			next++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if next-1 < before {
			t.Fatalf("read stopped at %d, but %d entries were confirmed before it started", next-1, before)
		}
		if err := j.Verify(); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := j.Last(); n != total {
		t.Fatalf("last entry is %d, want %d", n, total)
	}
}

func TestRealConfirmation(t *testing.T) {
	// One test with the real os.File.Sync, to cover the path the system uses.
	dir := t.TempDir()
	j, err := Open(dir, Options{SegmentSize: 512})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for i := 1; i <= 12; i++ {
		if _, err := j.Append(content(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
}

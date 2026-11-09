package journal

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const firstSegment = "00000000000000000001.journal"

// build writes n entries and returns the bytes of the only segment.
func build(t *testing.T, n int) []byte {
	t.Helper()
	dir := t.TempDir()
	j := open(t, dir, Options{})
	appendN(t, j, 1, n)
	j.Close()
	data, err := os.ReadFile(filepath.Join(dir, firstSegment))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// place writes data as the first segment of a new folder.
func place(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, firstSegment), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// lines splits a segment into its lines, each with its final newline.
func lines(data []byte) [][]byte {
	return bytes.SplitAfter(bytes.TrimSuffix(data, nil), []byte{'\n'})[:bytes.Count(data, []byte{'\n'})]
}

func wantDamage(t *testing.T, err error, entry uint64) {
	t.Helper()
	var damage *DamageError
	if !errors.As(err, &damage) {
		t.Fatalf("want a DamageError at entry %d, got %v", entry, err)
	}
	if damage.Entry != entry {
		t.Fatalf("damage reported at entry %d, want %d (%v)", damage.Entry, entry, err)
	}
}

func TestTornTailIsCutAtEveryByte(t *testing.T) {
	data := build(t, 3)
	all := lines(data)
	start := len(data) - len(all[2]) // where the last line begins
	for cut := start + 1; cut < len(data); cut++ {
		dir := place(t, data[:cut])
		j, err := Open(dir, Options{})
		if err != nil {
			t.Fatalf("cut at byte %d: %v", cut, err)
		}
		if n, _ := j.Last(); n != 2 {
			t.Fatalf("cut at byte %d: last entry is %d, want 2", cut, n)
		}
		j.sync = func(*os.File) error { return nil }
		if _, err := j.Append(content(3)); err != nil {
			t.Fatalf("cut at byte %d: append: %v", cut, err)
		}
		if err := j.Verify(); err != nil {
			t.Fatalf("cut at byte %d: verify: %v", cut, err)
		}
		j.Close()
		got, _ := os.ReadFile(filepath.Join(dir, firstSegment))
		if !bytes.Equal(got, data) {
			t.Fatalf("cut at byte %d: the journal did not come back to the same bytes", cut)
		}
	}
}

func TestTailOfZerosIsCut(t *testing.T) {
	data := build(t, 2)
	for _, zeros := range []int{1, 4096, MaxContent * 3} {
		dir := place(t, append(bytes.Clone(data), make([]byte, zeros)...))
		j := open(t, dir, Options{})
		if n, _ := j.Last(); n != 2 {
			t.Fatalf("%d zeros: last entry is %d, want 2", zeros, n)
		}
		appendN(t, j, 3, 1)
		if err := j.Verify(); err != nil {
			t.Fatalf("%d zeros: %v", zeros, err)
		}
		j.Close()
	}
}

func TestSegmentCutToNothingIsRemoved(t *testing.T) {
	data := build(t, 1)
	dir := place(t, data[:len(data)/2])
	j := open(t, dir, Options{})
	if n, _ := j.Last(); n != 0 {
		t.Fatalf("last entry is %d, want 0", n)
	}
	if files := segmentFiles(t, dir); len(files) != 0 {
		t.Fatalf("the empty segment was kept: %v", files)
	}
	appendN(t, j, 1, 2)
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyLastSegmentIsRemoved(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{SegmentSize: 256})
	appendN(t, j, 1, 10)
	last, seal := j.Last()
	j.Close()
	empty := filepath.Join(dir, "00000000000000000011.journal")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	j = open(t, dir, Options{SegmentSize: 256})
	if n, s := j.Last(); n != last || s != seal {
		t.Fatalf("last entry is %d, want %d", n, last)
	}
	appendN(t, j, 11, 3)
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestDamageInTheLastSegmentStopsOpen(t *testing.T) {
	data := build(t, 5)
	all := lines(data)
	offset := len(all[0]) // start of entry 2

	cases := map[string]struct {
		data  []byte
		entry uint64
	}{
		"content changed": {flip(data, offset+len(all[1])-5), 2},
		"header changed":  {flip(data, offset+30), 2},
		"line removed":    {join(all[0], all[1], all[3], all[4]), 3},
		"lines swapped":   {join(all[0], all[2], all[1], all[3], all[4]), 2},
		"line repeated":   {join(all[0], all[1], all[1], all[2]), 3},
		"garbage line":    {append(bytes.Clone(data), []byte("not an entry\n")...), 6},
		"valid after gap": {join(all[0], make([]byte, 300), []byte{'\n'}, all[2]), 2},
	}
	for name, c := range cases {
		dir := place(t, c.data)
		_, err := Open(dir, Options{})
		if err == nil {
			t.Fatalf("%s: the journal opened", name)
		}
		wantDamage(t, err, c.entry)
		after, _ := os.ReadFile(filepath.Join(dir, firstSegment))
		if !bytes.Equal(after, c.data) {
			t.Errorf("%s: Open changed a damaged journal", name)
		}
	}
}

func TestDamageInAnOlderSegmentIsFoundByVerify(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{SegmentSize: 512})
	appendN(t, j, 1, 40)
	j.Close()
	files := segmentFiles(t, dir)
	if len(files) < 3 {
		t.Fatalf("want at least 3 segments, got %d", len(files))
	}
	data, _ := os.ReadFile(files[0])
	first := lines(data)
	if err := os.WriteFile(files[0], flip(data, len(first[0])+len(first[1])-5), 0o600); err != nil {
		t.Fatal(err)
	}

	j = open(t, dir, Options{SegmentSize: 512}) // only the last segment is read here
	wantDamage(t, j.Verify(), 2)
	err := j.Read(1, func(Entry) error { return nil })
	wantDamage(t, err, 2)
}

func TestMissingSegmentIsFoundByVerify(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{SegmentSize: 512})
	appendN(t, j, 1, 40)
	j.Close()
	files := segmentFiles(t, dir)
	if err := os.Remove(files[1]); err != nil {
		t.Fatal(err)
	}
	j = open(t, dir, Options{SegmentSize: 512})
	var damage *DamageError
	if err := j.Verify(); !errors.As(err, &damage) {
		t.Fatalf("want a DamageError, got %v", err)
	}
}

func TestSegmentNameMustMatchItsFirstEntry(t *testing.T) {
	data := build(t, 3)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000007.journal"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(dir, Options{})
	wantDamage(t, err, 7)
}

func TestErasedEntryKeepsTheChain(t *testing.T) {
	data := build(t, 4)
	all := lines(data)
	header := all[1][:bytes.LastIndexByte(all[1], '\t')+1]
	erased := append(bytes.Clone(header), []byte(erasedMark+"\n")...)
	dir := place(t, join(all[0], erased, all[2], all[3]))

	j := open(t, dir, Options{})
	if err := j.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	entries := readAll(t, j, 1)
	if !entries[1].Erased || entries[1].Content != nil {
		t.Fatal("entry 2 does not read as erased")
	}
	if entries[2].Erased || entries[2].PrevSeal != entries[1].Seal() {
		t.Fatal("entry 3 lost its tie to the erased entry")
	}
	j.Close()

	// Erasing must not open the door to rewriting: a changed digest shows.
	tampered := flip(join(all[0], erased, all[2], all[3]), len(all[0])+len(header)-10)
	_, err := Open(place(t, tampered), Options{})
	if err == nil {
		t.Fatal("a changed digest in an erased entry went unnoticed")
	}
}

// flip returns a copy of data with one byte changed to another valid
// lowercase hex or letter character, so only the checks can notice.
func flip(data []byte, at int) []byte {
	out := bytes.Clone(data)
	if out[at] == 'a' {
		out[at] = 'b'
	} else {
		out[at] = 'a'
	}
	return out
}

func join(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

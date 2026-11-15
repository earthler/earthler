package journal

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
)

func TestACopyGrowsLikeTheOriginal(t *testing.T) {
	root := t.TempDir()
	original := open(t, filepath.Join(root, "original"), Options{})
	copied := open(t, filepath.Join(root, "copy"), Options{})

	written, err := original.Append([]byte(`{"n":1}`), []byte(`{"n":2}`), []byte(`{"n":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Erase(2); err != nil {
		t.Fatal(err)
	}
	// What travels is the line, read back from the original: entry 2 goes
	// erased, as the original holds it now.
	var lines [][]byte
	if err := original.Read(1, func(e Entry) error {
		lines = append(lines, Line(e))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var arrived []Entry
	for _, l := range lines {
		e, err := ParseLine(l)
		if err != nil {
			t.Fatal(err)
		}
		arrived = append(arrived, e)
	}
	if err := copied.Receive(arrived); err != nil {
		t.Fatal(err)
	}
	if n, seal := copied.Last(); n != 3 || seal != written[2].Seal() {
		t.Fatalf("the copy is at %d, %s", n, seal)
	}
	if err := copied.Verify(); err != nil {
		t.Fatal(err)
	}
	var got [][]byte
	copied.Read(1, func(e Entry) error {
		got = append(got, Line(e))
		return nil
	})
	if !bytes.Equal(bytes.Join(got, nil), bytes.Join(lines, nil)) {
		t.Fatal("the copy does not hold the same lines as the original")
	}

	// The copy only takes what follows it.
	more, err := original.Append([]byte(`{"n":4}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := copied.Receive(arrived[2:]); !errors.Is(err, ErrNotNext) {
		t.Fatalf("an entry the copy already has was taken again: %v", err)
	}
	forged := more[0]
	forged.PrevSeal = Seal{1}
	if err := copied.Receive([]Entry{forged}); !errors.Is(err, ErrNotNext) {
		t.Fatalf("an entry of another history was taken: %v", err)
	}
	changed := more[0]
	changed.Content = []byte(`{"n":5}`)
	if err := copied.Receive([]Entry{changed}); err == nil {
		t.Fatal("a content that does not match its digest was taken")
	}
	// A refused entry leaves the copy able to receive the right one.
	if err := copied.Receive(more); err != nil {
		t.Fatal(err)
	}
	if n, _ := copied.Last(); n != 4 {
		t.Fatalf("the copy is at %d", n)
	}
}

func TestACopySurvivesReopening(t *testing.T) {
	root := t.TempDir()
	original := open(t, filepath.Join(root, "original"), Options{})
	written, err := original.Append([]byte(`{"n":1}`), []byte(`{"n":2}`))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "copy")
	copied := open(t, dir, Options{})
	if err := copied.Receive(written); err != nil {
		t.Fatal(err)
	}
	copied.Close()
	again := open(t, dir, Options{})
	if n, seal := again.Last(); n != 2 || seal != written[1].Seal() {
		t.Fatalf("reopened at %d", n)
	}
}

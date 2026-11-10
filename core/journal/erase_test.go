package journal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEraseKeepsTheChainAndTheWriter(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{SegmentSize: 200}) // a few entries per segment
	appendN(t, j, 1, 12)
	if len(segmentFiles(t, dir)) < 3 {
		t.Fatalf("expected several segments, got %d", len(segmentFiles(t, dir)))
	}
	sealBefore := readAll(t, j, 1)[11].Seal()

	if err := j.Erase(3, 11, 12); err != nil {
		t.Fatalf("erase: %v", err)
	}
	entries := readAll(t, j, 1)
	for _, e := range entries {
		erased := e.Number == 3 || e.Number == 11 || e.Number == 12
		if e.Erased != erased || (erased && e.Content != nil) || (!erased && !bytes.Equal(e.Content, content(int(e.Number)))) {
			t.Fatalf("entry %d after erase: erased=%v content=%q", e.Number, e.Erased, e.Content)
		}
	}
	if entries[11].Seal() != sealBefore {
		t.Fatal("the seal of an erased entry changed")
	}
	if err := j.Verify(); err != nil {
		t.Fatalf("verify after erase: %v", err)
	}

	// The writer goes on in the rewritten last segment.
	added, err := j.Append(content(13))
	if err != nil || added[0].Number != 13 || added[0].PrevSeal != sealBefore {
		t.Fatalf("append after erase: %v %v", added, err)
	}
	if err := j.Verify(); err != nil {
		t.Fatalf("verify after append: %v", err)
	}
	if got := readAll(t, j, 13); len(got) != 1 || !bytes.Equal(got[0].Content, content(13)) {
		t.Fatalf("read after erase: %v", got)
	}

	// Erasing again changes nothing, and the files say so.
	path := segmentFiles(t, dir)[0]
	before, _ := os.ReadFile(path)
	if err := j.Erase(3); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("a repeated erase rewrote the segment")
	}
	if err := j.Erase(99); err == nil {
		t.Fatal("erasing an entry that does not exist should fail")
	}

	// The journal opens again with the erased entries still erased.
	j.Close()
	j = open(t, dir, Options{SegmentSize: 200})
	if n, _ := j.Last(); n != 13 {
		t.Fatalf("after reopening, last = %d", n)
	}
	if got := readAll(t, j, 3); !got[0].Erased || got[7].Erased || !got[8].Erased || !got[9].Erased || got[10].Erased {
		t.Fatalf("erased flags lost after reopening")
	}
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedEraseLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	j := open(t, dir, Options{})
	appendN(t, j, 1, 3)
	j.Close()
	leftover := filepath.Join(dir, "00000000000000000001.journal"+rewriteSuffix)
	if err := os.WriteFile(leftover, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	j = open(t, dir, Options{})
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatal("the leftover of an interrupted erase was kept")
	}
	if got := readAll(t, j, 1); len(got) != 3 || got[0].Erased {
		t.Fatalf("the journal changed: %v", got)
	}
}

func TestEraseRefusesAfterAFailedWrite(t *testing.T) {
	j := open(t, t.TempDir(), Options{})
	appendN(t, j, 1, 2)
	j.failed = os.ErrClosed
	if err := j.Erase(1); err == nil {
		t.Fatal("erase should refuse after a failed write")
	}
}

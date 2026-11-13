package journal

import "testing"

func TestSealAtSaysIfTwoMachinesHoldTheSameHistory(t *testing.T) {
	j := open(t, t.TempDir(), Options{SegmentSize: 300}) // several segments
	appendN(t, j, 1, 25)

	for _, n := range []uint64{1, 2, 13, 25} {
		got, err := j.SealAt(n)
		if err != nil {
			t.Fatalf("SealAt(%d): %v", n, err)
		}
		var want Seal
		for _, e := range readAll(t, j, n) {
			if e.Number == n {
				want = e.Seal()
				break
			}
		}
		if got != want {
			t.Fatalf("SealAt(%d) = %s, want %s", n, got, want)
		}
	}
	if last, seal := j.Last(); true {
		if got, err := j.SealAt(last); err != nil || got != seal {
			t.Fatalf("the seal at the last entry is %s, the journal says %s, %v", got, seal, err)
		}
	}
	if got, err := j.SealAt(0); err != nil || got != (Seal{}) {
		t.Fatalf("SealAt(0) = %s, %v", got, err)
	}
	if _, err := j.SealAt(26); err == nil {
		t.Fatal("the seal of an entry that was never written")
	}

	// Another journal with one different entry has another seal from there on.
	other := open(t, t.TempDir(), Options{SegmentSize: 300})
	appendN(t, other, 1, 12)
	other.Append([]byte(`{"n":999}`))
	appendN(t, other, 14, 12)
	a, _ := j.SealAt(12)
	b, _ := other.SealAt(12)
	c, _ := j.SealAt(20)
	d, _ := other.SealAt(20)
	if a != b || c == d {
		t.Fatalf("seals before the difference: %v, after: %v", a == b, c == d)
	}
}

package replica

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/journal"
)

// TestADamagedOrCutPullLeavesTheOldCopyWhole damages a file on its way, then
// cuts a pull in the middle. Each time the second machine must keep the last
// copy that it had whole, readable with the key, and say what went wrong.
func TestADamagedOrCutPullLeavesTheOldCopyWhole(t *testing.T) {
	var damage, cut atomic.Bool
	var served atomic.Int32
	m := newMainWith(t, 10, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/replica/files/") {
				if cut.Load() && served.Add(1) > 1 {
					http.Error(w, "gone", http.StatusInternalServerError)
					return
				}
				if damage.Load() {
					next.ServeHTTP(&flipper{ResponseWriter: w}, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	dir := filepath.Join(t.TempDir(), "second")
	f := m.pair(dir)
	if _, err := f.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}
	whole := func(when string) {
		t.Helper()
		if _, err := copies.Check(dir, m.key); err != nil {
			t.Fatalf("%s: the copy on the second machine is not whole: %v", when, err)
		}
		if anywhere(dir, ".tmp") {
			t.Fatalf("%s: a half downloaded file was left", when)
		}
	}
	whole("after the first pull")

	m.add(20)
	damage.Store(true)
	if _, err := f.Pull(context.Background()); err == nil || !strings.Contains(err.Error(), "did not arrive whole") {
		t.Fatalf("a damaged file: %v", err)
	}
	whole("after a damaged file")
	damage.Store(false)

	cut.Store(true)
	m.add(20)
	if _, err := f.Pull(context.Background()); err == nil {
		t.Fatal("a pull whose files stopped coming did not fail")
	}
	whole("after a cut pull")

	// A leftover of a pull that was killed is cleared by the next good one.
	stray := filepath.Join(dir, "0123456789abcdef0123456789abcdef.tmp")
	os.WriteFile(stray, []byte("half"), 0o600)
	cut.Store(false)
	if _, err := f.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Fatal("the leftover of a killed pull is still there")
	}
	whole("after the good pull")
}

// flipper changes the first byte of what it writes.
type flipper struct {
	http.ResponseWriter
	done bool
}

func (f *flipper) Write(p []byte) (int, error) {
	if !f.done && len(p) > 0 {
		q := append([]byte(nil), p...)
		q[0] ^= 0xff
		f.done = true
		return f.ResponseWriter.Write(q)
	}
	return f.ResponseWriter.Write(p)
}

// TestAHistoryThatIsNotTheOneFollowedIsSetAside makes the main machine look
// like it was restored from something older, or like another journal. The
// second machine must keep what it had in a folder of its own, take the new
// history in full, and delete nothing.
func TestAHistoryThatIsNotTheOneFollowedIsSetAside(t *testing.T) {
	for name, claim := range map[string]struct {
		last uint64
		seal string
	}{
		"older":           {last: 9999, seal: strings.Repeat("ab", 32)},
		"another journal": {last: 5, seal: strings.Repeat("00", 32)},
	} {
		t.Run(name, func(t *testing.T) {
			m := newMain(t, 12)
			dir := filepath.Join(t.TempDir(), "second")
			f := m.pair(dir)
			if _, err := f.Pull(context.Background()); err != nil {
				t.Fatal(err)
			}
			before := filesOf(t, dir)
			f.state.Last, f.state.Seal = claim.last, claim.seal

			done, err := f.Pull(context.Background())
			if err != nil || !done.Diverged {
				t.Fatalf("pull: %+v, %v", done, err)
			}
			aside, _ := filepath.Glob(filepath.Join(dir, divergedDir, "*", "*"))
			if len(aside) != len(before) {
				t.Fatalf("%d files were set aside, there were %d", len(aside), len(before))
			}
			if _, err := copies.Check(dir, m.key); err != nil {
				t.Fatalf("the new history is not whole: %v", err)
			}
			if again, err := f.Pull(context.Background()); err != nil || again.Diverged {
				t.Fatalf("the next pull: %+v, %v", again, err)
			}
		})
	}
}

func filesOf(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && copies.IsOurs(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names
}

// TestAnErasureReachesTheSecondMachine erases an entry on the main machine.
// The old file of the journal is replaced on the second machine too, so a
// text that was deleted is not kept in a copy that is somewhere else.
func TestAnErasureReachesTheSecondMachine(t *testing.T) {
	m := newMain(t, 20)
	dir := filepath.Join(t.TempDir(), "second")
	f := m.pair(dir)
	if _, err := f.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := filesOf(t, dir)

	if err := m.journal.Erase(1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := filesOf(t, dir)
	gone := 0
	have := map[string]bool{}
	for _, n := range after {
		have[n] = true
	}
	for _, n := range before {
		if !have[n] {
			gone++
		}
	}
	if gone < 1 {
		t.Fatalf("no file was replaced on the second machine after an erasure: %v and %v", before, after)
	}

	taken := filepath.Join(t.TempDir(), "taken")
	if _, err := copies.Restore(dir, taken, m.key); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(filepath.Join(taken, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	erased := false
	j.Read(1, func(e journal.Entry) error {
		if e.Number == 1 {
			erased = e.Erased
		}
		return nil
	})
	if !erased || j.Verify() != nil {
		t.Fatalf("after taking over, entry 1 erased: %v, journal verifies: %v", erased, j.Verify() == nil)
	}
}

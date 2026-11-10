package scribe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
)

var notes = index.Area{
	Name:   "notes",
	Schema: `CREATE TABLE notes (number INTEGER PRIMARY KEY, text TEXT NOT NULL);`,
	Apply: func(tx *sql.Tx, e journal.Entry) error {
		var data struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(e.Content, &data); err != nil {
			return nil // an entry this index does not understand is not its business
		}
		_, err := tx.Exec(`INSERT INTO notes (number, text) VALUES (?, ?)`, e.Number, data.Text)
		return err
	},
}

type fixture struct {
	root    string
	journal *journal.Journal
	index   *index.Index
	scribe  *Scribe
}

func start(t *testing.T, root string) *fixture {
	t.Helper()
	j, err := journal.Open(filepath.Join(root, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(root, "indexes"), notes)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Start(j, ix)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{root: root, journal: j, index: ix, scribe: s}
	t.Cleanup(f.stop)
	return f
}

func (f *fixture) stop() {
	f.scribe.Close()
	f.index.Close()
	f.journal.Close()
}

func note(text string) []byte {
	c, _ := json.Marshal(map[string]string{"text": text})
	return c
}

func count(t *testing.T, ix *index.Index) int {
	t.Helper()
	var n int
	if err := ix.DB().QueryRow(`SELECT count(*) FROM notes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWriteIsVisibleWhenItReturns(t *testing.T) {
	f := start(t, t.TempDir())
	entries, err := f.scribe.Write(context.Background(), note("one"), note("two"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Number != 1 || entries[1].Number != 2 {
		t.Fatalf("entries: %v", entries)
	}
	var text string
	if err := f.index.DB().QueryRow(`SELECT text FROM notes WHERE number = 2`).Scan(&text); err != nil || text != "two" {
		t.Fatalf("the index did not have the entry when Write returned: %q, %v", text, err)
	}
	if last, _ := f.journal.Last(); last != 2 {
		t.Fatalf("journal ends at %d", last)
	}
}

func TestConcurrentWritersGetConsecutiveEntries(t *testing.T) {
	f := start(t, t.TempDir())
	const writers, each = 40, 10
	var mu sync.Mutex
	seen := map[uint64]bool{}
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				entries, err := f.scribe.Write(context.Background(), note(fmt.Sprintf("%d-%d-a", w, i)), note(fmt.Sprintf("%d-%d-b", w, i)))
				if err != nil {
					errs <- err
					return
				}
				if len(entries) != 2 || entries[1].Number != entries[0].Number+1 {
					errs <- fmt.Errorf("one write got entries that are not consecutive: %v", entries)
					return
				}
				mu.Lock()
				seen[entries[0].Number], seen[entries[1].Number] = true, true
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	const total = writers * each * 2
	if len(seen) != total {
		t.Fatalf("%d different numbers for %d entries", len(seen), total)
	}
	if last, _ := f.journal.Last(); last != total {
		t.Fatalf("journal ends at %d, want %d", last, total)
	}
	if err := f.journal.Verify(); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f.index); n != total {
		t.Fatalf("the index has %d rows, want %d", n, total)
	}
}

func TestBadContentFailsOnlyItsRequest(t *testing.T) {
	f := start(t, t.TempDir())
	if _, err := f.scribe.Write(context.Background(), []byte(`not json`)); err == nil {
		t.Fatal("bad content was accepted")
	}
	if _, err := f.scribe.Write(context.Background()); err == nil {
		t.Fatal("an empty write was accepted")
	}
	if _, err := f.scribe.Write(context.Background(), note("fine")); err != nil {
		t.Fatalf("a good write failed after a bad one: %v", err)
	}
	if n := count(t, f.index); n != 1 {
		t.Fatalf("the index has %d rows, want 1", n)
	}
}

func TestStartBringsIndexesLevel(t *testing.T) {
	root := t.TempDir()
	j, err := journal.Open(filepath.Join(root, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(note("before one"), note("before two")); err != nil {
		t.Fatal(err)
	}
	j.Close()

	f := start(t, root)
	if n := count(t, f.index); n != 2 {
		t.Fatalf("the index has %d rows after Start, want 2", n)
	}
	entries, err := f.scribe.Write(context.Background(), note("after"))
	if err != nil || entries[0].Number != 3 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
}

func TestIndexFailureStopsTheScribe(t *testing.T) {
	root := t.TempDir()
	f := start(t, root)
	if _, err := f.scribe.Write(context.Background(), note("one")); err != nil {
		t.Fatal(err)
	}
	f.index.Close() // the index stops taking writes, as with a full or broken disk

	if _, err := f.scribe.Write(context.Background(), note("two")); !errors.Is(err, ErrStopped) {
		t.Fatalf("want ErrStopped, got %v", err)
	}
	if _, err := f.scribe.Write(context.Background(), note("three")); !errors.Is(err, ErrStopped) {
		t.Fatalf("the scribe kept writing after an index failed: %v", err)
	}
	if f.scribe.Err() == nil {
		t.Fatal("the scribe does not say why it stopped")
	}
	f.stop()

	// The entry reached the journal before the index failed. A restart
	// brings the index level, so nothing confirmed by the disk is missing.
	again := start(t, root)
	if last, _ := again.journal.Last(); last != 2 {
		t.Fatalf("journal ends at %d, want 2", last)
	}
	if n := count(t, again.index); n != 2 {
		t.Fatalf("the index has %d rows after the restart, want 2", n)
	}
}

func TestClosedScribeRefusesWrites(t *testing.T) {
	f := start(t, t.TempDir())
	f.scribe.Close()
	if _, err := f.scribe.Write(context.Background(), note("late")); !errors.Is(err, ErrStopped) {
		t.Fatalf("want ErrStopped, got %v", err)
	}
	if f.scribe.Err() != nil {
		t.Fatalf("a normal Close left an error: %v", f.scribe.Err())
	}
}

func TestListenersHearEachBatchBeforeTheAnswer(t *testing.T) {
	f := start(t, t.TempDir())
	heard := make(chan []journal.Entry, 8)
	answered := false
	f.scribe.Listen(func(entries []journal.Entry) {
		if answered {
			t.Error("the listener ran after the writer got its answer")
		}
		// The index already has the entries when a listener hears of them.
		if n := count(t, f.index); n < int(entries[len(entries)-1].Number) {
			t.Errorf("the index has %d notes when the listener hears of entry %d", n, entries[len(entries)-1].Number)
		}
		heard <- entries
	})
	entries, err := f.scribe.Write(context.Background(), note("one"), note("two"))
	answered = true
	if err != nil {
		t.Fatal(err)
	}
	got := <-heard
	if len(got) != 2 || got[0].Number != entries[0].Number || got[1].Number != entries[1].Number {
		t.Fatalf("the listener heard %v, want %v", got, entries)
	}
}

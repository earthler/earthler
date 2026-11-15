package scribe

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
)

// byID keeps one row per entry, by id, whatever machine wrote it.
var byID = index.Area{
	Name:   "byid",
	Schema: `CREATE TABLE seen (id INTEGER PRIMARY KEY);`,
	Apply: func(tx *sql.Tx, e index.Entry) error {
		_, err := tx.Exec(`INSERT INTO seen (id) VALUES (?)`, e.ID)
		return err
	},
}

func said(t *testing.T, at time.Time, text string) []byte {
	t.Helper()
	c, err := event.New(event.MessageCreated, 3, 3, at, event.MessageCreatedData{Channel: 2, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func openAt(t *testing.T, dir string) *journal.Journal {
	t.Helper()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	return j
}

func TestTheClockNeverGoesBack(t *testing.T) {
	root := t.TempDir()
	j := openAt(t, filepath.Join(root, "journal"))
	// The machine's clock was ahead an hour when it wrote this, and then
	// someone set it right.
	ahead := time.Now().Add(time.Hour)
	if _, err := j.Append(said(t, ahead, "written with the clock ahead")); err != nil {
		t.Fatal(err)
	}
	s, err := Start(j)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	entries, err := s.Write(context.Background(), said(t, time.Now(), "written later"))
	if err != nil {
		t.Fatal(err)
	}
	// Inside one journal the same time is enough: the entry number orders.
	if got := event.TimeOf(entries[0].Content); got != ahead.UnixMilli() {
		t.Fatalf("the new entry has time %d, before the %d already written", got, ahead.UnixMilli())
	}
}

func TestABatchNeverGoesBack(t *testing.T) {
	j := openAt(t, filepath.Join(t.TempDir(), "journal"))
	s, err := Start(j)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Two screens stamped their contents in the other order, and the two
	// went into one batch.
	later, earlier := time.UnixMilli(200_000), time.UnixMilli(150_000)
	entries, err := s.Write(context.Background(), said(t, later, "a"), said(t, earlier, "b"))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := event.TimeOf(entries[0].Content), event.TimeOf(entries[1].Content); b < a {
		t.Fatalf("the second entry has time %d, before the first, %d", b, a)
	}
}

func TestTimeComesAfterWhatOtherMachinesWrote(t *testing.T) {
	root := t.TempDir()
	mine := openAt(t, filepath.Join(root, "journal"))
	theirs := openAt(t, filepath.Join(root, "journals", "2"))
	// Machine 2's clock is ahead of this one.
	ahead := time.Now().Add(time.Hour)
	if _, err := theirs.Append(said(t, ahead, "a question, from machine 2")); err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(root, "indexes"), byID)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	s, err := Open(Journals{Self: 1, Own: mine, Kept: map[uint64]*journal.Journal{2: theirs}}, ix)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	entries, err := s.Write(context.Background(), said(t, time.Now(), "the answer, from machine 1"))
	if err != nil {
		t.Fatal(err)
	}
	// Across journals the order is the time, so the answer must come after
	// the question even though this machine's clock is behind.
	if got := event.TimeOf(entries[0].Content); got != ahead.UnixMilli()+1 {
		t.Fatalf("the answer has time %d, the question %d", got, ahead.UnixMilli())
	}
	if entries[0].Machine != 1 || entries[0].ID != event.ID(1, 1) {
		t.Fatalf("the entry is %d of machine %d", entries[0].ID, entries[0].Machine)
	}
	// The index has both machines' entries, each under its own id.
	var rows int
	if err := ix.DB().QueryRow(`SELECT count(*) FROM seen WHERE id IN (?, ?)`, event.ID(1, 1), event.ID(2, 1)).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("the index has %d of the 2 entries (%v)", rows, err)
	}
}

func TestAMachineHoldsItsJournalsOnce(t *testing.T) {
	root := t.TempDir()
	mine := openAt(t, filepath.Join(root, "journal"))
	other := openAt(t, filepath.Join(root, "journals", "1"))
	for _, js := range []Journals{
		{Self: 1, Own: mine, Kept: map[uint64]*journal.Journal{1: other}},
		{Self: event.MaxMachine + 1, Own: mine},
		{Self: 0, Own: mine, Kept: map[uint64]*journal.Journal{event.MaxMachine + 1: other}},
		{Self: 0, Own: mine, Kept: map[uint64]*journal.Journal{2: nil}},
		{Self: 0},
	} {
		if s, err := Open(js); err == nil {
			s.Close()
			t.Errorf("started with %+v", js)
		}
	}
}

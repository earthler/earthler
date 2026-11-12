package erasure

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/scribe"
)

// texts is a small index that keeps every message, to see erasure from the
// side of an index that is rebuilt.
var texts = index.Area{
	Name:   "texts",
	Schema: `CREATE TABLE texts (number INTEGER PRIMARY KEY, text TEXT NOT NULL);`,
	Apply: func(tx *sql.Tx, entry journal.Entry) error {
		e, err := event.Decode(entry.Content)
		if err != nil || e.Type != event.MessageCreated {
			return nil
		}
		var d event.MessageCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO texts (number, text) VALUES (?, ?)`, entry.Number, d.Text)
		return err
	},
}

func TestDeletedEntriesAreErasedOnceAndStayErased(t *testing.T) {
	root := t.TempDir()
	j, err := journal.Open(filepath.Join(root, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	open := func() (*index.Index, *index.Index, *scribe.Scribe) {
		pending, err := index.Open(filepath.Join(root, "indexes"), Area)
		if err != nil {
			t.Fatal(err)
		}
		kept, err := index.Open(filepath.Join(root, "indexes"), texts)
		if err != nil {
			t.Fatal(err)
		}
		s, err := scribe.Start(j, pending, kept)
		if err != nil {
			t.Fatal(err)
		}
		return pending, kept, s
	}
	pending, kept, s := open()
	now := time.Now()
	ctx := context.Background()
	write := func(typ string, data any) uint64 {
		content, err := event.New(typ, 1, 1, now, data)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := s.Write(ctx, content)
		if err != nil {
			t.Fatal(err)
		}
		return entries[0].Number
	}
	first := write(event.MessageCreated, event.MessageCreatedData{Channel: 1, Text: "fica"})
	second := write(event.MessageCreated, event.MessageCreatedData{Channel: 1, Text: "some"})
	write(event.MessageDeleted, event.MessageDeletedData{Channel: 1, Message: second})

	if n, err := Run(ctx, j, pending, s, now); err != nil || n != 1 {
		t.Fatalf("run: %d, %v", n, err)
	}
	if n, err := Run(ctx, j, pending, s, now); err != nil || n != 0 {
		t.Fatalf("second run: %d, %v", n, err)
	}
	var count int
	pending.DB().QueryRow(`SELECT count(*) FROM pending`).Scan(&count)
	if count != 0 {
		t.Fatalf("%d entries still pending", count)
	}
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
	var erased, total int
	j.Read(1, func(e journal.Entry) error {
		total++
		if e.Erased {
			erased++
			if e.Number != second {
				t.Errorf("entry %d is erased", e.Number)
			}
		}
		return nil
	})
	if total != 4 || erased != 1 {
		t.Fatalf("%d entries, %d erased", total, erased)
	}

	// Rebuilt from the journal, the indexes agree: the text is gone, the
	// other one stays, and nothing is pending.
	s.Close()
	pending.Close()
	kept.Close()
	if err := os.RemoveAll(filepath.Join(root, "indexes")); err != nil {
		t.Fatal(err)
	}
	pending, kept, s = open()
	defer s.Close()
	defer pending.Close()
	defer kept.Close()
	var text string
	if err := kept.DB().QueryRow(`SELECT text FROM texts WHERE number = ?`, first).Scan(&text); err != nil || text != "fica" {
		t.Fatalf("the kept text: %q, %v", text, err)
	}
	if err := kept.DB().QueryRow(`SELECT text FROM texts WHERE number = ?`, second).Scan(&text); err == nil {
		t.Fatalf("the erased text came back: %q", text)
	}
	pending.DB().QueryRow(`SELECT count(*) FROM pending`).Scan(&count)
	if count != 0 {
		t.Fatalf("%d entries pending after a rebuild", count)
	}
}

func TestNextNight(t *testing.T) {
	loc := time.FixedZone("test", -3*3600)
	now := time.Date(2026, 11, 10, 14, 30, 0, 0, loc)
	if got := NextNight(now, 3); !got.Equal(time.Date(2026, 11, 11, 3, 0, 0, 0, loc)) {
		t.Fatalf("next night from the afternoon: %v", got)
	}
	early := time.Date(2026, 11, 10, 2, 59, 0, 0, loc)
	if got := NextNight(early, 3); !got.Equal(time.Date(2026, 11, 10, 3, 0, 0, 0, loc)) {
		t.Fatalf("next night from before the hour: %v", got)
	}
}

// TestEverythingPendingIsErasedInOneRun deletes more messages than fit in one
// batch. The night's copy comes right after the run, so a run that stopped
// at the first batch would put texts that were deleted into the copy.
func TestEverythingPendingIsErasedInOneRun(t *testing.T) {
	defer func(old int) { batch = old }(batch)
	batch = 7

	root := t.TempDir()
	j, err := journal.Open(filepath.Join(root, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	pending, err := index.Open(filepath.Join(root, "indexes"), Area)
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Close()
	s, err := scribe.Start(j, pending)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now, ctx := time.Now(), context.Background()

	const messages = 30
	var numbers []uint64
	for i := 0; i < messages; i++ {
		content, _ := event.New(event.MessageCreated, 1, 1, now, event.MessageCreatedData{Channel: 1, Text: "segredo de " + string(rune('a'+i%26))})
		entries, err := s.Write(ctx, content)
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, entries[0].Number)
	}
	for _, n := range numbers {
		content, _ := event.New(event.MessageDeleted, 1, 1, now, event.MessageDeletedData{Channel: 1, Message: n})
		if _, err := s.Write(ctx, content); err != nil {
			t.Fatal(err)
		}
	}

	if n, err := Run(ctx, j, pending, s, now); err != nil || n != messages {
		t.Fatalf("one run erased %d of %d, %v", n, messages, err)
	}
	var left int
	if err := pending.DB().QueryRow(`SELECT count(*) FROM pending`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d entries are still pending, %v", left, err)
	}
	for _, path := range func() []string { m, _ := filepath.Glob(filepath.Join(root, "journal", "*.journal")); return m }() {
		raw, _ := os.ReadFile(path)
		if strings.Contains(string(raw), "segredo de") {
			t.Fatalf("a deleted text is still in %s", path)
		}
	}
	if err := j.Verify(); err != nil {
		t.Fatal(err)
	}
}

package erasure

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/scribe"
)

func content(t *testing.T, typ string, data any) []byte {
	t.Helper()
	c, err := event.New(typ, 3, 3, time.Now(), data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func countPending(t *testing.T, ix *index.Index) []uint64 {
	t.Helper()
	rows, err := ix.DB().Query(`SELECT entry FROM pending ORDER BY entry`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []uint64
	for rows.Next() {
		var id uint64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// A machine erases in the copies it keeps, and another machine's record of
// erasing the same entry does not do its work for it.
func TestEachMachineErasesItsOwnCopies(t *testing.T) {
	root := t.TempDir()
	own, err := journal.Open(filepath.Join(root, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	kept, err := journal.Open(filepath.Join(root, "journals", "1"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer kept.Close()

	// On machine 1, a message was written, deleted, and erased there. Machine
	// 0 holds the copy it received before the erasure, text and all.
	message := event.ID(1, 1)
	if _, err := kept.Append(
		content(t, event.MessageCreated, event.MessageCreatedData{Channel: 2, Text: "apaga"}),
		content(t, event.MessageDeleted, event.MessageDeletedData{Channel: 2, Message: message}),
		content(t, event.EntryErased, event.EntryErasedData{Entries: []uint64{message}}),
	); err != nil {
		t.Fatal(err)
	}
	pending, err := index.Open(filepath.Join(root, "indexes"), Area(0))
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Close()
	journals := map[uint64]*journal.Journal{0: own, 1: kept}
	s, err := scribe.Open(scribe.Journals{Self: 0, Own: own, Kept: map[uint64]*journal.Journal{1: kept}}, pending)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := countPending(t, pending); len(got) != 1 || got[0] != message {
		t.Fatalf("pending on machine 0: %v", got)
	}

	// A deletion of a message whose journal this machine does not hold
	// waits for that journal.
	far := event.ID(7, 3)
	if _, err := s.Write(context.Background(), content(t, event.MessageDeleted, event.MessageDeletedData{Channel: 2, Message: far})); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if n, err := Run(ctx, journals, pending, s, time.Now()); err != nil || n != 1 {
		t.Fatalf("run erased %d, %v", n, err)
	}
	if got := countPending(t, pending); len(got) != 1 || got[0] != far {
		t.Fatalf("pending after the run: %v", got)
	}
	err = kept.Read(1, func(e journal.Entry) error {
		if !e.Erased {
			t.Error("the copy of machine 1 still has the text")
		}
		return errStop
	})
	if err != nil && !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if n, err := Run(ctx, journals, pending, s, time.Now()); err != nil || n != 0 {
		t.Fatalf("a second run erased %d, %v", n, err)
	}
}

var errStop = errors.New("stop")

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

// A copy that is behind does not stop the erasure of everything else, and a
// pending index of another machine is an error, not a run without end.
func TestErasureWaitsForACopyThatIsBehind(t *testing.T) {
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
	if _, err := kept.Append(content(t, event.MessageCreated, event.MessageCreatedData{Channel: 2, Text: "um"})); err != nil {
		t.Fatal(err)
	}
	pending, err := index.Open(filepath.Join(root, "indexes"), Area(0))
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Close()
	s, err := scribe.Open(scribe.Journals{Own: own, Kept: map[uint64]*journal.Journal{1: kept}}, pending)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	mine, err := s.Write(ctx, content(t, event.MessageCreated, event.MessageCreatedData{Channel: 2, Text: "meu"}))
	if err != nil {
		t.Fatal(err)
	}
	ahead := event.ID(1, 5) // machine 1 wrote it, and the copy does not have it yet
	for _, id := range []uint64{mine[0].ID, ahead} {
		if _, err := s.Write(ctx, content(t, event.MessageDeleted, event.MessageDeletedData{Channel: 2, Message: id})); err != nil {
			t.Fatal(err)
		}
	}
	journals := map[uint64]*journal.Journal{0: own, 1: kept}
	if n, err := Run(ctx, journals, pending, s, time.Now()); err != nil || n != 1 {
		t.Fatalf("run erased %d, %v", n, err)
	}
	if got := countPending(t, pending); len(got) != 1 || got[0] != ahead {
		t.Fatalf("pending: %v", got)
	}

	// The index of machine 1, with the scribe of machine 0.
	other, err := index.Open(filepath.Join(root, "other"), Area(1))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.Follow(journals); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(ctx, content(t, event.MessageDeleted, event.MessageDeletedData{Channel: 2, Message: event.ID(1, 1)})); err != nil {
		t.Fatal(err)
	}
	if err := other.Follow(journals); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, journals, other, s, time.Now())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a run with another machine's index ended without an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a run with another machine's index did not end")
	}
}

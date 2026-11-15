package scribe

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
)

func TestEntriesFromAnotherMachineReachTheIndexes(t *testing.T) {
	root := t.TempDir()
	mine := openAt(t, filepath.Join(root, "journal"))
	// What machine 2 wrote in its own journal, somewhere else.
	original := openAt(t, filepath.Join(root, "elsewhere"))
	ahead := time.Now().Add(time.Hour)
	theirs, err := original.Append(said(t, ahead, "um"), said(t, ahead, "dois"))
	if err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(root, "indexes"), byID)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	s, err := Start(mine, ix)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	heard := make(chan []index.Entry, 4)
	s.Listen(func(entries []index.Entry) { heard <- entries })
	ctx := context.Background()

	if err := s.Receive(ctx, 2, theirs); err == nil {
		t.Fatal("entries of a machine whose journal is not kept were taken")
	}
	copyDir := filepath.Join(root, "journals", "2")
	if err := s.Keep(ctx, 2, openAt(t, copyDir)); err != nil {
		t.Fatal(err)
	}
	if err := s.Keep(ctx, 2, openAt(t, filepath.Join(root, "again"))); err == nil {
		t.Fatal("a second copy of the same machine was kept")
	}
	if err := s.Keep(ctx, 0, openAt(t, filepath.Join(root, "self"))); err == nil {
		t.Fatal("a machine kept a copy of its own journal")
	}

	// Entry 2 first does not follow the copy, and the scribe goes on.
	if err := s.Receive(ctx, 2, theirs[1:]); !errors.Is(err, journal.ErrNotNext) {
		t.Fatalf("entry 2 before entry 1: %v", err)
	}
	if err := s.Receive(ctx, 2, theirs); err != nil {
		t.Fatal(err)
	}
	got := <-heard
	if len(got) != 2 || got[0].Machine != 2 || got[1].ID != event.ID(2, 2) {
		t.Fatalf("the listener heard %+v", got)
	}
	var rows int
	ix.DB().QueryRow(`SELECT count(*) FROM seen WHERE id IN (?, ?)`, event.ID(2, 1), event.ID(2, 2)).Scan(&rows)
	if rows != 2 {
		t.Fatalf("the index has %d of machine 2's entries", rows)
	}

	// What this machine writes next comes after what it received.
	entries, err := s.Write(ctx, said(t, time.Now(), "três"))
	if err != nil {
		t.Fatal(err)
	}
	if got := event.TimeOf(entries[0].Content); got != ahead.UnixMilli()+1 {
		t.Fatalf("written at %d, after receiving %d", got, ahead.UnixMilli())
	}
}

func TestAKeptCopyIsAppliedWhenKept(t *testing.T) {
	root := t.TempDir()
	mine := openAt(t, filepath.Join(root, "journal"))
	copied := openAt(t, filepath.Join(root, "journals", "5"))
	original := openAt(t, filepath.Join(root, "elsewhere"))
	theirs, err := original.Append(said(t, time.Now(), "já estava aqui"))
	if err != nil {
		t.Fatal(err)
	}
	if err := copied.Receive(theirs); err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(root, "indexes"), byID)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	s, err := Start(mine, ix)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Keep(context.Background(), 5, copied); err != nil {
		t.Fatal(err)
	}
	if ix.Last(5) != 1 {
		t.Fatalf("the index is at entry %d of machine 5", ix.Last(5))
	}
}

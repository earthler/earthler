package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/earthler/earthler/core/copies"
)

// TestACopyBringsTheCommunityBackOnAnotherMachine is the proof of the
// copies: what residents wrote comes back, in another folder, from the
// encrypted copy and the key written down.
func TestACopyBringsTheCommunityBackOnAnotherMachine(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data")
	s := startSite(t, dir)
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	ana.say(s, garden)
	s.stop()

	key, err := copies.LoadKey(filepath.Join(dir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "disk")
	if err := makeCopy(dir, dest); err != nil {
		t.Fatal(err)
	}
	if err := restore(filepath.Join(root, "data"), dest, key.Text()); err == nil {
		t.Fatal("restoring over the live data was accepted")
	}
	if err := restore(filepath.Join(root, "new-machine"), dest, "not a key"); err == nil {
		t.Fatal("a wrong key was accepted")
	}
	if err := restore(filepath.Join(root, "new-machine"), dest, key.Text()); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "new-machine", "indexes")); len(entries) != 0 {
		t.Fatal("the copy carried indexes")
	}

	s = startSite(t, filepath.Join(root, "new-machine"))
	_, body := ana.get(s, "/chat")
	wantIn(t, body, garden, "the conversation on the new machine")
	wantIn(t, body, "Ana", "the session on the new machine, signed with the same key")
	if err := s.app.journal.Verify(); err != nil {
		t.Fatal(err)
	}
}

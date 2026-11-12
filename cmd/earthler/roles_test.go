package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/earthler/earthler/core/event"
)

// give has an administrator give a role to an account, the way the screen
// of residents does.
func (v *visitor) give(s *site, account uint64, role string) (int, string) {
	return v.post(s, "/admin/residents/role", s.srv.URL, url.Values{"number": {strconv.FormatUint(account, 10)}, "role": {role}})
}

// removeWith removes a message that is not one's own, giving the reason.
func (v *visitor) removeWith(s *site, number, reason string) (int, string) {
	return v.post(s, "/chat/delete", s.srv.URL, url.Values{"number": {number}, "reason": {reason}})
}

func entries(t *testing.T, s *site) uint64 {
	t.Helper()
	n, _ := s.app.journal.Last()
	return n
}

// TestTheFirstResidentLooksAfterTheCommunity checks who sees the list of
// residents: the first account, which is the administrator, and nobody else.
func TestTheFirstResidentLooksAfterTheCommunity(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")

	status, body := ana.get(s, "/admin/residents")
	if status != http.StatusOK {
		t.Fatalf("the administrator reading the residents: status %d", status)
	}
	wantIn(t, body, "Ana", "the list of residents")
	wantIn(t, body, "Bia", "the list of residents")
	wantIn(t, body, `href="/admin/residents"`, "the navigation of the administrator")

	status, body = bia.get(s, "/admin/residents")
	if status != http.StatusForbidden {
		t.Fatalf("a resident reading the list: status %d", status)
	}
	_, body = bia.get(s, "/chat")
	wantOut(t, body, `href="/admin/residents"`, "the navigation of a resident")
	wantIn(t, body, `href="/log"`, "the navigation of a resident")
}

// TestRolesAreGivenByAdministratorsAndRememberedAfterARebuild goes through
// the rules of a change of role, and checks the result survives the loss of
// every index.
func TestRolesAreGivenByAdministratorsAndRememberedAfterARebuild(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)
	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")
	before := entries(t, s)

	// A resident cannot give a role, not even to themselves.
	for _, account := range []uint64{3, 4} {
		if status, _ := bia.give(s, account, "administrator"); status != http.StatusForbidden {
			t.Errorf("a resident giving a role to %d: status %d", account, status)
		}
	}
	// Roles that do not exist, or are not given by hand, and accounts that do not exist.
	for _, role := range []string{"", "root", "Administrator", "child", " moderator"} {
		if status, _ := ana.give(s, 4, role); status != http.StatusBadRequest {
			t.Errorf("giving the role %q: status %d, want 400", role, status)
		}
	}
	for _, account := range []uint64{0, 1, 2, 99} {
		if status, _ := ana.give(s, account, "moderator"); status != http.StatusNotFound {
			// Entries 1 and 2 exist but are not accounts.
			t.Errorf("giving a role to %d, which is not an account: status %d, want 404", account, status)
		}
	}
	// The last administrator cannot step down.
	if status, body := ana.give(s, 3, "resident"); status != http.StatusConflict {
		t.Fatalf("the only administrator giving up the role: status %d", status)
	} else {
		wantIn(t, body, "pelo menos um administrador", "the refusal")
	}
	if after := entries(t, s); after != before {
		t.Fatalf("refused changes wrote %d entries", after-before)
	}

	// A real change, and the same change again, which writes nothing.
	if status, _ := ana.give(s, 4, "moderator"); status != http.StatusOK {
		t.Fatalf("making Bia a moderator: status %d", status)
	}
	changed := entries(t, s)
	ana.give(s, 4, "moderator")
	if entries(t, s) != changed {
		t.Fatal("giving the role someone already has wrote an entry")
	}
	_, body := ana.get(s, "/admin/residents")
	wantIn(t, body, `<option value="moderator" selected>`, "the list after the change")
	_, body = bia.get(s, "/log")
	wantIn(t, body, "Ana deu o papel de Moderador a Bia.", "the log read by the one who got the role")

	// With a second administrator, the first may step down.
	ana.give(s, 4, "administrator")
	stepping := entries(t, s)
	// The answer is the list of residents, which the one who stepped down
	// may no longer read, so the proof is the entry and what comes after it.
	ana.give(s, 3, "resident")
	if entries(t, s) != stepping+1 {
		t.Fatal("an administrator could not step down once there was another")
	}
	if status, _ := ana.get(s, "/admin/residents"); status != http.StatusForbidden {
		t.Fatalf("the one who stepped down still reads the list: status %d", status)
	}

	// Every index is thrown away. The roles come back from the journal.
	s.stop()
	if err := os.RemoveAll(filepath.Join(dir, "indexes")); err != nil {
		t.Fatal(err)
	}
	s = startSite(t, dir)
	if status, _ := bia.get(s, "/admin/residents"); status != http.StatusOK {
		t.Fatalf("after a rebuild, the new administrator reading the list: status %d", status)
	}
	if status, _ := ana.get(s, "/admin/residents"); status != http.StatusForbidden {
		t.Fatalf("after a rebuild, the former administrator reading the list: status %d", status)
	}
	_, body = ana.get(s, "/log")
	wantIn(t, body, "Ana deu o papel de Administrador a Bia.", "the log after a rebuild")
	wantIn(t, body, "Ana deu o papel de Morador a Ana.", "the log after a rebuild")
}

// TestAnEntryThatSkipsTheScreenStillChangesNothing writes the event of a
// change of role the way a bug, or someone with the files, could: from an
// account that is not an administrator. The indexes must ignore it.
func TestAnEntryThatSkipsTheScreenStillChangesNothing(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")

	for _, data := range []event.RoleChangedData{{Account: 4, Role: "administrator"}, {Account: 3, Role: "resident"}, {Account: 4, Role: "child"}, {Account: 4, Role: "root"}} {
		content, _ := event.New(event.RoleChanged, 4, 4, time.Now(), data) // written as Bia
		if _, err := s.app.scribe.Write(context.Background(), content); err != nil {
			t.Fatal(err)
		}
	}
	if status, _ := bia.get(s, "/admin/residents"); status != http.StatusForbidden {
		t.Fatalf("Bia made herself an administrator: status %d", status)
	}
	if status, _ := ana.get(s, "/admin/residents"); status != http.StatusOK {
		t.Fatalf("Ana lost her role: status %d", status)
	}
	_, body := ana.get(s, "/log")
	wantOut(t, body, "Bia deu o papel", "the log, which is read from the same entries")
}

package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestModeratorsRemoveWithAReasonAndTheLogShowsIt is the proof of roles in
// the open channel: a moderator or an administrator removes what someone
// else wrote, only with a reason, a resident cannot, and the act is in the
// log that every resident reads.
func TestModeratorsRemoveWithAReasonAndTheLogShowsIt(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana, bia, caio, dani := newVisitor(t), newVisitor(t), newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")
	caio.signUp(s, "Caio")
	dani.signUp(s, "Dani")
	ana.give(s, 5, "moderator") // Caio
	// Entries: 1 community, 2 channel, 3 Ana, 4 Bia, 5 Caio, 6 Dani, 7 the role.

	bia.say(s, "Compre meu adubo, chama no privado")
	bia.say(s, "Bom dia, pessoal")
	bia.say(s, "Outra propaganda")
	// 8, 9 and 10 are Bia's messages.

	// The page of a moderator offers the removal of what is not hers, and
	// the page of a resident does not.
	_, body := caio.get(s, "/chat")
	if got := strings.Count(body, `name="reason"`); got != 4 { // three messages and the template for new ones
		t.Fatalf("the moderator's page has %d reason fields, want 4", got)
	}
	_, body = dani.get(s, "/chat")
	wantOut(t, body, `name="reason"`, "the page of a resident")

	before := entries(t, s)
	// A resident cannot, with or without a reason.
	for _, reason := range []string{"", "propaganda"} {
		if status, _ := dani.removeWith(s, "8", reason); status != http.StatusForbidden {
			t.Errorf("a resident removing with the reason %q: status %d, want 403", reason, status)
		}
	}
	// A moderator needs a reason, and a sensible one.
	for _, reason := range []string{"", "   ", strings.Repeat("a", 201), "ruim\xff"} {
		if status, _ := caio.removeWith(s, "8", reason); status != http.StatusBadRequest {
			t.Errorf("a moderator removing with the reason %q: status %d, want 400", reason, status)
		}
	}
	if entries(t, s) != before {
		t.Fatal("a refused removal wrote an entry")
	}

	if status, body := caio.removeWith(s, "8", "Propaganda em canal aberto"); status != http.StatusOK {
		t.Fatalf("a moderator removing with a reason: status %d", status)
	} else {
		wantOut(t, body, "Compre meu adubo", "the channel after the removal")
		wantIn(t, body, "Mensagem apagada.", "the channel after the removal")
	}
	if status, _ := ana.removeWith(s, "10", "Mais propaganda"); status != http.StatusOK {
		t.Fatalf("an administrator removing with a reason: status %d", status)
	}
	// The author may still delete without a reason, and it is not a power.
	if status, _ := bia.remove(s, "9"); status != http.StatusOK {
		t.Fatalf("the author deleting: status %d", status)
	}

	_, body = dani.get(s, "/log")
	wantIn(t, body, "Caio removeu uma mensagem de Bia. Motivo: Propaganda em canal aberto", "the log read by a resident")
	wantIn(t, body, "Ana removeu uma mensagem de Bia. Motivo: Mais propaganda", "the log read by a resident")
	if got := strings.Count(body, "removeu uma mensagem"); got != 2 {
		t.Fatalf("the log has %d removals, want 2: the author's own deletion is not one", got)
	}
	_, body = dani.get(s, "/search?q=adubo")
	wantOut(t, body, "Compre meu adubo", "the search after the removal")
}

// TestAccountsThatOnlyReadDoNotWrite gives an account the role of a child,
// which no menu can do yet, and checks that it reads the channel and cannot
// write in it.
func TestAccountsThatOnlyReadDoNotWrite(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana, kid := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	kid.signUp(s, "Tomás")
	ana.say(s, "Quem vem à horta hoje?")
	if _, err := s.app.byName["accounts"].DB().Exec(`UPDATE roster SET role = 'child' WHERE account = 4`); err != nil {
		t.Fatal(err)
	}

	_, body := kid.get(s, "/chat")
	wantIn(t, body, "Quem vem à horta hoje?", "what a child reads")
	wantIn(t, body, "Esta conta só lê as conversas.", "the page of a child")
	wantOut(t, body, `<textarea`, "the page of a child")
	before := entries(t, s)
	if status, _ := kid.say(s, "oi"); status != http.StatusForbidden {
		t.Fatalf("a child writing: status %d, want 403", status)
	}
	if entries(t, s) != before {
		t.Fatal("a child's message reached the journal")
	}
	if status, _ := ana.give(s, 4, "administrator"); status != http.StatusForbidden {
		t.Fatalf("giving a role to a child by hand: status %d, want 403", status)
	}
	_, body = kid.get(s, "/log")
	wantIn(t, body, "Registro", "the log, which a child may read")
}

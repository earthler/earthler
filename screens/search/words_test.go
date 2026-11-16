package search

import (
	"strings"
	"testing"

	terms "github.com/earthler/earthler/core/search"
)

// TestTheWordFoundIsMarked marks every word with the root that was searched
// for, whatever its form, and lets nothing in the text become markup.
func TestTheWordFoundIsMarked(t *testing.T) {
	roots := map[string]bool{}
	for _, r := range terms.Terms("Plantas") {
		roots[r] = true
	}
	for text, want := range map[string]string{
		"Vou plantar amanhã":       "Vou <mark>plantar</mark> amanhã",
		"PLANTAS, planta e plano.": "<mark>PLANTAS</mark>, <mark>planta</mark> e plano.",
		"<b>planta</b> & cia":      "&lt;b&gt;<mark>planta</mark>&lt;/b&gt; &amp; cia",
		"nada aqui":                "nada aqui",
	} {
		if got := string(marked(text, roots)); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
}

// TestALongTextShowsTheWordFound cuts a long message around the first word
// found, so the card shows it however far into the message it is.
func TestALongTextShowsTheWordFound(t *testing.T) {
	roots := map[string]bool{}
	for _, r := range terms.Terms("bomba") {
		roots[r] = true
	}
	long := strings.Repeat("palavra ", 60) + "a bomba parou " + strings.Repeat("depois ", 60)
	got := excerpt(long, roots, 400)
	if !strings.Contains(string(marked(got, roots)), "<mark>bomba</mark>") {
		t.Fatalf("the word found is not on the card: %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") || len([]rune(got)) != 402 {
		t.Fatalf("the cut is not marked on both sides: %q", got)
	}
	if short := "a bomba parou"; excerpt(short, roots, 400) != short {
		t.Fatal("a short text was cut")
	}
	if got := excerpt(strings.Repeat("x", 500)+" bomba", roots, 400); !strings.HasSuffix(got, "bomba") {
		t.Fatalf("the word at the very end is not on the card: %q", got[len(got)-20:])
	}
}

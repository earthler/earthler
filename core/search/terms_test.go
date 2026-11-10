package search

import (
	"slices"
	"strings"
	"testing"
)

func TestSameRootSameTerm(t *testing.T) {
	groups := [][]string{
		{"planta", "plantas", "Plantar", "PLANTAS"},
		{"água", "agua", "Águas"},
		{"semente", "sementes"},
	}
	for _, g := range groups {
		want := Terms(g[0])
		if len(want) != 1 {
			t.Fatalf("%q gave %v", g[0], want)
		}
		for _, word := range g[1:] {
			if got := Terms(word); !slices.Equal(got, want) {
				t.Errorf("%q gave %v, but %q gave %v", word, got, g[0], want)
			}
		}
	}
	// For the record: derived words do not meet, and words with a nasal
	// ending may not meet their unaccented spelling. A list of synonyms is
	// the planned answer for both.
	for _, word := range []string{"plantio", "plantação", "reunião", "reuniao", "reuniões"} {
		t.Logf("%s -> %v", word, Terms(word))
	}
}

func TestDifferentWordsStayApart(t *testing.T) {
	if slices.Equal(Terms("horta"), Terms("porta")) {
		t.Error("horta and porta became the same term")
	}
}

func TestTextIsSplitIntoWords(t *testing.T) {
	got := Terms("A horta, hoje: 3 canteiros (novos)!")
	if len(got) != 6 {
		t.Fatalf("got %d terms: %v", len(got), got)
	}
	for _, term := range got {
		for _, r := range term {
			if r > 127 {
				t.Errorf("term %q still has an accent", term)
			}
		}
	}
	if len(Terms("  ... !!! ")) != 0 {
		t.Error("punctuation alone gave terms")
	}
}

func TestQueryRequiresEveryTerm(t *testing.T) {
	terms := Terms("água da horta")
	if len(terms) != 3 {
		t.Fatalf("terms = %v", terms)
	}
	if got, want := Query("água da horta"), `"`+strings.Join(terms, `" "`)+`"`; got != want {
		t.Errorf("query = %s, want %s", got, want)
	}

	// Whatever the person types, nothing may reach the index as an operator.
	q := Query(`"OR" NEAR(x) * horta AND -y`)
	for _, part := range strings.Fields(q) {
		if len(part) < 3 || part[0] != '"' || part[len(part)-1] != '"' || strings.Count(part, `"`) != 2 {
			t.Errorf("part %s of the query %s is not a quoted term", part, q)
		}
	}
	if strings.ContainsAny(q, "()*-") {
		t.Errorf("the query kept operator characters: %s", q)
	}
	if Query("   ") != "" {
		t.Error("an empty search gave a query")
	}
}

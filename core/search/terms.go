// Package search turns text into the terms that the search index stores and
// looks for. The same function prepares what is indexed and what is asked,
// so the two always meet.
package search

import (
	"strings"
	"unicode"

	"github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/portuguese"
)

// Terms splits text into words and reduces each one to a form that ignores
// case, accents and the usual endings of Portuguese: "plantas", "Plantar"
// and "planta" all become the same term.
//
// The full text search of SQLite only knows how to do this for English,
// which is why it is done here, before the text reaches the index.
func Terms(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, 0, len(words))
	for _, w := range words {
		if t := term(w); t != "" {
			terms = append(terms, t)
		}
	}
	return terms
}

// term reduces one lowercase word. The stemmer needs the accents to tell
// endings apart, so they are removed only afterwards.
func term(word string) string {
	env := snowballstem.NewEnv(word)
	portuguese.Stem(env)
	return unaccent(env.Current())
}

// plain maps the accented letters of Portuguese, and the few others that
// show up in names, to the plain letter.
var plain = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

func unaccent(s string) string {
	return strings.Map(func(r rune) rune {
		if p, ok := plain[r]; ok {
			return p
		}
		return r
	}, s)
}

// Query writes terms as a full text query that requires all of them. Each
// term goes in quotes so nothing in it is read as an operator.
func Query(text string) string {
	terms := Terms(text)
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, ``) + `"`
	}
	return strings.Join(quoted, " ")
}

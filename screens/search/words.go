package search

import (
	"html/template"
	"net/url"
	"strings"
	"unicode"

	terms "github.com/earthler/earthler/core/search"
)

func known(in string) bool {
	for _, key := range filters {
		if key == in {
			return true
		}
	}
	return false
}

func name(key string) string {
	if key == "" {
		return "all"
	}
	return key
}

// link is the address of the same words in another filter.
func link(asked, in string) string {
	v := url.Values{"q": {asked}}
	if in != "" {
		v.Set("in", in)
	}
	return "/search?" + v.Encode()
}

// marked escapes the text and puts every word whose root was searched for
// inside a mark, so "plantar" is marked when "plantas" was typed.
func marked(text string, roots map[string]bool) template.HTML {
	var b strings.Builder
	var word []rune
	flush := func() {
		if len(word) == 0 {
			return
		}
		w := string(word)
		if t := terms.Terms(w); len(t) == 1 && roots[t[0]] {
			b.WriteString("<mark>" + template.HTMLEscapeString(w) + "</mark>")
		} else {
			b.WriteString(template.HTMLEscapeString(w))
		}
		word = word[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word = append(word, r)
			continue
		}
		flush()
		b.WriteString(template.HTMLEscapeString(string(r)))
	}
	flush()
	return template.HTML(b.String())
}

// excerpt cuts a long text to about n letters, around the first word
// found, so the word shows on the card however far into the text it is.
func excerpt(text string, roots map[string]bool, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	at, start := -1, -1
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if t := terms.Terms(string(runes[start:i])); len(t) == 1 && roots[t[0]] {
				at = start
				break
			}
			start = -1
		}
	}
	if at < 0 && start >= 0 {
		if t := terms.Terms(string(runes[start:])); len(t) == 1 && roots[t[0]] {
			at = start
		}
	}
	from := 0
	if at > n/3 {
		from = at - n/3
	}
	if from+n > len(runes) {
		from = len(runes) - n
	}
	out := string(runes[from : from+n])
	if from > 0 {
		out = "…" + out
	}
	if from+n < len(runes) {
		out += "…"
	}
	return out
}

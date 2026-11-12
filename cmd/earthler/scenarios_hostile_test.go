package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Scenarios where someone sends what a person with a text editor or a script
// could send.

// TestCookiesThatLookRightButAreNot takes a good cookie and bends it every
// way a person with a text editor could. None of them gets in.
func TestCookiesThatLookRightButAreNot(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	bia := newVisitor(t)
	bia.signUp(s, "Bia")

	u, _ := url.Parse(s.srv.URL)
	good := ana.client.Jar.Cookies(u)[0].Value
	parts := strings.Split(good, ".")
	if len(parts) != 3 {
		t.Fatalf("the cookie %q does not have three parts", good)
	}
	bad := map[string]string{
		"counter changed":       parts[0] + ".1." + parts[2],
		"device of someone":     "4." + parts[1] + "." + parts[2],
		"signature in capitals": parts[0] + "." + parts[1] + "." + strings.ToUpper(parts[2]),
		"signature plus junk":   good + "00",
		"an extra part":         good + ".1",
		"leading zeros":         "0" + parts[0] + "." + parts[1] + "." + parts[2],
		"negative":              "-" + parts[0] + "." + parts[1] + "." + parts[2],
		"huge":                  "99999999999999999999999." + parts[1] + "." + parts[2],
		"empty":                 "",
		"only dots":             "..",
		"no signature":          parts[0] + "." + parts[1] + ".",
		"spaces":                " " + good,
	}
	for name, value := range bad {
		forger := newVisitor(t)
		forger.client.Jar.SetCookies(u, []*http.Cookie{{Name: "earthler", Value: value}})
		if _, body := forger.get(s, "/chat"); !strings.Contains(body, "Criar conta") {
			t.Errorf("%s: the cookie %q was accepted", name, value)
		}
	}
}

// TestNamesAreNeverPartOfThePage signs up with names that carry markup and
// with characters that cannot be seen. Markup is shown as text. A name that
// looks like another resident's name is refused.
func TestNamesAreNeverPartOfThePage(t *testing.T) {
	s := startSite(t, t.TempDir())
	evil := newVisitor(t)
	status, body := evil.signUp(s, `<img src=x onerror=alert(1)>`)
	if status != http.StatusOK {
		t.Fatalf("a name with markup: status %d", status)
	}
	wantOut(t, body, `<img src=x`, "the page of someone with markup in the name")
	wantIn(t, body, `&lt;img src=x`, "the page of someone with markup in the name")

	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	for name, text := range map[string]string{
		"a zero width space":       "Ana\u200b",
		"a right to left override": "Ana\u202e",
		"a soft hyphen":            "A\u00adna",
		"a word joiner":            "An\u2060a",
		"a byte order mark":        "\ufeffAna",
		"a private use character":  "Ana\ue000",
		"a lone surrogate":         "Ana\xed\xa0\x80",
		"an invalid byte":          "Ana\xff",
	} {
		other := newVisitor(t)
		if status, _ := other.signUp(s, text); status == http.StatusOK {
			t.Errorf("%s: the name %q was accepted", name, text)
		}
	}
}

// TestSearchSurvivesStrangeQueries throws operators, quotes, controls and
// very long text at the search field. It must always answer a page.
func TestSearchSurvivesStrangeQueries(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	ana.say(s, garden)

	queries := []string{
		"", " ", "*", "**", `"`, `""`, `"horta`, `horta"`, "horta*", "hort*", "^horta", "-horta", "+horta",
		"horta AND", "OR horta", "NOT", "NEAR(horta agua, 2)", "NEAR(", "(horta", "horta)", "{horta}", "horta:agua",
		"text:horta", "terms:horta", "rank:1", "horta' OR '1'='1", "horta; DROP TABLE docs;--", "' --", "%", "_", "\\",
		"horta\x00agua", "\x00", "\xff\xfe", "\xed\xa0\x80", "água", "ÁGUA", "ａｇｕａ", "🌱", "\u202ehorta",
		strings.Repeat("horta ", 2000), strings.Repeat("a", 100000), strings.Repeat("(", 5000), strings.Repeat(`"`, 5001),
	}
	for _, q := range queries {
		status, _ := ana.get(s, "/search?q="+url.QueryEscape(q))
		if status != http.StatusOK && status != http.StatusBadRequest && status != http.StatusRequestURITooLong {
			label := q
			if len(label) > 40 {
				label = label[:40] + "..."
			}
			t.Errorf("search for %q: status %d", label, status)
		}
	}
	// A normal search still works afterwards.
	_, body := ana.get(s, "/search?q=horta")
	wantIn(t, body, garden, "a search after the strange ones")
}

// TestDeletingTheWrongThing sends every kind of bad request to the delete
// route. Nothing but the author's own message may ever be deleted.
func TestDeletingTheWrongThing(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")
	ana.say(s, "da Ana")
	// Entries: 1 community, 2 channel, 3 Ana, 4 Bia, 5 message.
	before, _ := s.app.journal.Last()

	for _, number := range []string{"", "x", "-1", "0", "1.5", "5 ", " 5", "05x", "99999999999999999999", "18446744073709551615", "9223372036854775808", "3", "1", "2", "4", "5", "6", "100"} {
		status, _ := bia.remove(s, number)
		if status < 400 {
			t.Errorf("Bia deleting %q: status %d, want an error", number, status)
		}
	}
	if status, _ := newVisitor(t).remove(s, "5"); status >= 400 && status != http.StatusForbidden {
		// A stranger is sent to the sign up screen, which answers 200 after the redirect.
		t.Logf("a stranger deleting: status %d", status)
	}
	for _, number := range []string{"3", "1", "2", "4", "6", "100", "0"} {
		if status, _ := ana.remove(s, number); status < 400 {
			t.Errorf("Ana deleting %q, which is not her message: status %d", number, status)
		}
	}
	if after, _ := s.app.journal.Last(); after != before {
		t.Fatalf("bad deletions wrote %d entries", after-before)
	}
	_, body := bia.get(s, "/chat")
	wantIn(t, body, "da Ana", "the message after every bad deletion")

	if status, _ := ana.remove(s, "5"); status >= 400 {
		t.Fatalf("Ana deleting her own message: status %d", status)
	}
	again, _ := s.app.journal.Last()
	ana.remove(s, "5")
	ana.remove(s, "5")
	if after, _ := s.app.journal.Last(); after != again {
		t.Fatalf("deleting a deleted message again wrote %d entries", after-again)
	}
}

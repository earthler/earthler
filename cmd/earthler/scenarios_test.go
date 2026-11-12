package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The scenarios in these files go past the proof of each stage. They ask what
// happens when many residents act at once, when the input is hostile, when
// the server stops in the middle of a write, and when the indexes are thrown
// away after a busy day.

// The scenarios in this file go past the proof of each stage. They ask what
// happens when many residents act at once, when the input is hostile, when
// the server stops in the middle of a write, and when the indexes are thrown
// away after a busy day.

// quiet is a browser that does not follow redirects, so a test can tell a
// message that was accepted (303) from one that was refused.
func quiet(t *testing.T) *visitor {
	v := newVisitor(t)
	v.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return v
}

// chatDump is every message in the index, in order, as text.
func chatDump(t *testing.T, s *site) string {
	t.Helper()
	rows, err := s.app.byName["chat"].DB().Query(`SELECT number, account, time, text, deleted FROM messages ORDER BY number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var number, account, deleted uint64
		var at, text string
		if err := rows.Scan(&number, &account, &at, &text, &deleted); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d|%d|%s|%d|%s", number, account, at, deleted, text))
	}
	return strings.Join(out, "\n")
}

func accountsDump(t *testing.T, s *site) string {
	t.Helper()
	rows, err := s.app.byName["accounts"].DB().Query(`SELECT a.number, a.name, coalesce(d.number, 0), coalesce(d.counter, 0) FROM accounts a LEFT JOIN devices d ON d.account = a.number ORDER BY a.number, d.number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var number, device, counter uint64
		var name string
		if err := rows.Scan(&number, &name, &device, &counter); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d|%s|%d|%d", number, name, device, counter))
	}
	return strings.Join(out, "\n")
}

// firstDifference shows the first line where two dumps part ways.
func firstDifference(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(x) || i < len(y); i++ {
		var l, r string
		if i < len(x) {
			l = x[i]
		}
		if i < len(y) {
			r = y[i]
		}
		if l != r {
			return fmt.Sprintf("line %d: kept up to date %q, rebuilt %q", i+1, l, r)
		}
	}
	return "no difference"
}

// TestManyResidentsAtOnce has twenty residents sign up and write at the same
// moment. Every message must be in the journal and the screens exactly once,
// the chain must hold, and indexes rebuilt from the journal must say the
// same thing as the ones that were kept up to date while it happened.
func TestManyResidentsAtOnce(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)

	const residents, each = 20, 15
	var wg sync.WaitGroup
	for r := 0; r < residents; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			v := newVisitor(t)
			if status, _ := v.signUp(s, fmt.Sprintf("Morador %02d", r)); status != http.StatusOK {
				t.Errorf("resident %d: sign up status %d", r, status)
				return
			}
			for m := 0; m < each; m++ {
				if status, _ := v.say(s, fmt.Sprintf("mensagem %02d-%02d", r, m)); status != http.StatusOK {
					t.Errorf("resident %d, message %d: status %d", r, m, status)
				}
			}
		}(r)
	}
	wg.Wait()
	if t.Failed() {
		return
	}

	want := 2 + residents + residents*each // community, channel, one entry for each resident, messages
	if last, _ := s.app.journal.Last(); last != uint64(want) {
		t.Fatalf("the journal has %d entries, want %d", last, want)
	}
	if err := s.app.journal.Verify(); err != nil {
		t.Fatal(err)
	}
	liveChat, liveAccounts := chatDump(t, s), accountsDump(t, s)
	if got := strings.Count(liveChat, "\n") + 1; got != residents*each {
		t.Fatalf("the chat index holds %d messages, want %d", got, residents*each)
	}

	s.stop()
	if err := os.RemoveAll(filepath.Join(dir, "indexes")); err != nil {
		t.Fatal(err)
	}
	s = startSite(t, dir)
	if got := chatDump(t, s); got != liveChat {
		t.Fatal("the chat index rebuilt from the journal differs from the one kept up to date")
	}
	if got := accountsDump(t, s); got != liveAccounts {
		t.Fatalf("the accounts index rebuilt from the journal differs from the one kept up to date:\n%s", firstDifference(liveAccounts, got))
	}
}

// TestOnlyOneAnaGetsTheName has the same name asked for at the same moment,
// written in different ways. One account is created, everyone else is told
// the name is taken.
func TestOnlyOneAnaGetsTheName(t *testing.T) {
	s := startSite(t, t.TempDir())
	names := []string{"Ana", "ana", "ANA", " Ana ", "Ana  ", "aNa", "Ana", "ANA"}
	var mu sync.Mutex
	statuses := map[int]int{}
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			status, _ := newVisitor(t).signUp(s, name)
			mu.Lock()
			statuses[status]++
			mu.Unlock()
		}(name)
	}
	wg.Wait()
	if statuses[http.StatusOK] != 1 || statuses[http.StatusConflict] != len(names)-1 {
		t.Fatalf("statuses %v: want one 200 and %d 409", statuses, len(names)-1)
	}
	if got := accountsDump(t, s); strings.Count(got, "\n") != 0 {
		t.Fatalf("more than one account was created:\n%s", got)
	}
}

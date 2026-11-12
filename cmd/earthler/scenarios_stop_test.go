package main

import (
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/earthler/earthler/core/journal"
)

// Scenarios where the server, or the person's connection, goes away in the
// middle of something.

// TestLiveConnectionsDoNotLeak opens and drops many live connections. When
// they are gone, so are the goroutines that served them.
func TestLiveConnectionsDoNotLeak(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()

	for i := 0; i < 60; i++ {
		_, stop := ana.listen(s, "0")
		stop()
	}
	var after int
	for waited := 0; waited < 50; waited++ {
		after = runtime.NumGoroutine()
		if after <= before+4 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("goroutines went from %d to %d after 60 live connections were dropped", before, after)
}

// TestStoppingInTheMiddleOfWriting stops the server while residents are
// still writing. Every message the server said yes to must be there when it
// comes back, the chain must hold, and nothing may be there twice.
func TestStoppingInTheMiddleOfWriting(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)

	const writers = 8
	visitors := make([]*visitor, writers)
	for i := range visitors {
		visitors[i] = quiet(t)
		if status, _ := visitors[i].signUp(s, fmt.Sprintf("Escritor %d", i)); status != http.StatusSeeOther {
			t.Fatalf("writer %d: sign up status %d", i, status)
		}
	}

	var mu sync.Mutex
	var accepted []string
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i, v := range visitors {
		wg.Add(1)
		go func(i int, v *visitor) {
			defer wg.Done()
			for n := 0; ; n++ {
				select {
				case <-stop:
					return
				default:
				}
				text := fmt.Sprintf("escritor %d, mensagem %d", i, n)
				req, _ := http.NewRequest("POST", s.srv.URL+"/chat", strings.NewReader(url.Values{"text": {text}}.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Origin", s.srv.URL)
				res, err := v.client.Do(req)
				if err != nil {
					return
				}
				res.Body.Close()
				if res.StatusCode == http.StatusSeeOther && res.Header.Get("Location") == "/chat#end" {
					mu.Lock()
					accepted = append(accepted, text)
					mu.Unlock()
				}
			}
		}(i, v)
	}
	time.Sleep(300 * time.Millisecond)
	s.app.Close() // the server goes down with writers still at it
	close(stop)
	wg.Wait()
	s.srv.Close()

	if len(accepted) == 0 {
		t.Fatal("no message was accepted before the stop, so the test proved nothing")
	}
	a, err := open(config{dataDir: dir, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.journal.Verify(); err != nil {
		t.Fatal(err)
	}
	have := map[string]int{}
	rows, err := a.byName["chat"].DB().Query(`SELECT text FROM messages`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		rows.Scan(&text)
		have[text]++
	}
	sort.Strings(accepted)
	for _, text := range accepted {
		if have[text] != 1 {
			inJournal := 0
			a.journal.Read(1, func(e journal.Entry) error {
				if strings.Contains(string(e.Content), text) {
					inJournal++
				}
				return nil
			})
			last, _ := a.journal.Last()
			t.Fatalf("%q was accepted and is in the chat %d times after the restart (in the journal: %d times, journal has %d entries)", text, have[text], inJournal, last)
		}
	}
	t.Logf("%d messages were accepted before the stop and all of them came back", len(accepted))
}

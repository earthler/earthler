package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/machines"
	"github.com/earthler/earthler/core/replica"
)

// TestARestoredMachineTakesItsJournalBack restores machine 0 from a copy
// older than its journal. Machine 1 holds the end that the copy lacks, and
// machine 0 takes it back before it writes anything, so the two keep one
// history and machine 1 goes on following it.
func TestARestoredMachineTakesItsJournalBack(t *testing.T) {
	first, most := machines.FirstWait, machines.MaxWait
	machines.FirstWait, machines.MaxWait = 20*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { machines.FirstWait, machines.MaxWait = first, most })

	dir0, dir1 := t.TempDir(), t.TempDir()
	cfg0 := config{dataDir: dir0, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443}
	a0, err := open(cfg0)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := a0.chain.Issue(time.Now(), []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	running := map[int]*app{0: a0}
	serving := func(n int) func() http.Handler {
		return func() http.Handler {
			mu.Lock()
			defer mu.Unlock()
			if a := running[n]; a != nil {
				return a.handler
			}
			return nil
		}
	}
	secure0, secure1 := newSecure(t, *cert), newSecure(t, *cert)
	secure0.handler, secure1.handler = serving(0), serving(1)
	welcome0 := httptest.NewServer(a0.welcome)
	t.Cleanup(welcome0.Close)
	stop := func(n int) {
		mu.Lock()
		a := running[n]
		running[n] = nil
		mu.Unlock()
		if a != nil {
			a.Hush()
			a.Close()
		}
	}
	t.Cleanup(func() { stop(0); stop(1) })

	s0 := &site{t: t, app: a0, srv: httptest.NewServer(a0.handler)}
	ana := newVisitor(t)
	ana.signUp(s0, "Ana")
	ana.say(s0, "Antes da cópia")
	code, err := replica.NewCode(filepath.Join(dir0, "machines"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cfg1 := config{dataDir: dir1, short: "x", lang: "pt-BR", httpsPort: 443}
	if err := join(cfg1, joinOptions{url: secure0.url(), plainURL: welcome0.URL, code: code,
		fingerprint: a0.chain.Fingerprint(), reach: secure1.url()}); err != nil {
		t.Fatal(err)
	}
	a1, err := open(cfg1)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	running[1] = a1
	mu.Unlock()

	// The night's copy of machine 0, and then a morning it does not have.
	key, err := copies.LoadKey(filepath.Join(dir0, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	night := t.TempDir()
	if _, err := copies.Copy(dir0, night, key, time.Now()); err != nil {
		t.Fatal(err)
	}
	ana.say(s0, "Depois da cópia, um")
	ana.say(s0, "Depois da cópia, dois")
	lost, _ := a0.journal.Last()
	eventually(t, "machine 1 holding all of machine 0's journal", func() bool {
		held := a1.holding()[0]
		n, _ := held.Last()
		return n == lost
	})

	// Machine 0's disk dies. It comes back from the night's copy.
	s0.srv.Close()
	stop(0)
	restored := filepath.Join(t.TempDir(), "restored")
	if _, err := copies.Restore(night, restored, key); err != nil {
		t.Fatal(err)
	}
	cfg0.dataDir = restored
	back, err := open(cfg0)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	running[0] = back
	mu.Unlock()
	if n, _ := back.journal.Last(); n != lost {
		t.Fatalf("the restored machine 0 is at entry %d of its journal, it had %d", n, lost)
	}
	s0 = &site{t: t, app: back, srv: httptest.NewServer(back.handler)}
	t.Cleanup(s0.srv.Close)
	_, body := ana.get(s0, "/chat")
	wantIn(t, body, "Depois da cópia, dois", "the restored machine 0")

	// And it goes on as one history: machine 1 takes what it writes next.
	ana.say(s0, "Voltei inteira")
	s1 := &site{t: t, app: a1, srv: httptest.NewServer(a1.handler)}
	t.Cleanup(s1.srv.Close)
	eventually(t, "machine 1 following the restored machine 0", func() bool {
		_, body := ana.get(s1, "/chat")
		return strings.Contains(body, "Voltei inteira")
	})
}

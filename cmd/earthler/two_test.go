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

// TestTwoMachinesOneCommunity is the proof of the second image: a machine
// joins with a code, takes the community's secrets, and from then on the two
// show the same conversation, whichever machine each resident uses. When
// one goes away, the other goes on, and what is counted says not now; when
// it comes back, each gets what it missed.
func TestTwoMachinesOneCommunity(t *testing.T) {
	machines.FirstWait, machines.MaxWait = 20*time.Millisecond, 200*time.Millisecond
	first, most := machines.FirstWait, machines.MaxWait
	t.Cleanup(func() { machines.FirstWait, machines.MaxWait = first, most })

	dir0, dir1 := t.TempDir(), t.TempDir()
	a0, err := open(config{dataDir: dir0, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443, label: "casa da Ana"})
	if err != nil {
		t.Fatal(err)
	}
	s0 := &site{t: t, app: a0, srv: httptest.NewServer(a0.handler)}
	t.Cleanup(func() { a0.Hush(); s0.stop() })
	cert, err := a0.chain.Issue(time.Now(), []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatal(err)
	}
	secure0, secure1 := newSecure(t, *cert), newSecure(t, *cert)
	secure0.handler = func() http.Handler { return a0.handler }
	welcome0 := httptest.NewServer(a0.welcome)
	t.Cleanup(welcome0.Close)

	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s0, "Ana")
	ana.say(s0, "Bom dia, primeira mensagem")

	// The second machine joins with the code and the fingerprint.
	code, err := replica.NewCode(filepath.Join(dir0, "machines"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cfg1 := config{dataDir: dir1, community: "Outro nome", short: "outro", lang: "pt-BR", httpsPort: 443, label: "casa da Bia"}
	if err := join(cfg1, joinOptions{url: secure0.url(), plainURL: welcome0.URL, code: code,
		fingerprint: a0.chain.Fingerprint(), reach: secure1.url()}); err != nil {
		t.Fatal(err)
	}
	if err := join(cfg1, joinOptions{url: secure0.url(), plainURL: welcome0.URL, code: code,
		fingerprint: a0.chain.Fingerprint(), reach: secure1.url()}); err == nil {
		t.Fatal("a folder that already joined joined again")
	}
	a1, err := open(cfg1)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	current1 := a1
	secure1.handler = func() http.Handler {
		mu.Lock()
		defer mu.Unlock()
		if current1 == nil {
			return nil
		}
		return current1.handler
	}
	s1 := &site{t: t, app: a1, srv: httptest.NewServer(a1.handler)}
	t.Cleanup(func() {
		s1.app.Hush()
		s1.srv.Close()
		mu.Lock()
		current1 = nil
		mu.Unlock()
		s1.app.Close()
	})

	if a1.self() != 1 || a1.name != "Vila Teste" || a1.chain.Fingerprint() != a0.chain.Fingerprint() {
		t.Fatalf("machine %d of %q, with fingerprint %s", a1.self(), a1.name, a1.chain.Fingerprint())
	}

	// Ana, signed up on machine 0, is signed in on machine 1 too: the same
	// key signs the cookies, and her account came with the journal.
	_, body := ana.get(s1, "/chat")
	wantIn(t, body, "Bom dia, primeira mensagem", "machine 1, for a resident of machine 0")

	// Bia signs up on machine 1. A name is counted, so machine 0 takes it.
	if status, body := bia.signUp(s1, "Bia"); status != http.StatusOK {
		t.Fatalf("sign up on machine 1: %d\n%s", status, body)
	}
	if status, _ := newVisitor(t).signUp(s1, "bia"); status != http.StatusConflict {
		t.Fatalf("a name taken through machine 1 was taken again: %d", status)
	}
	bia.say(s1, "Oi, escrevo da casa da Bia")
	ana.say(s0, "Respondo da casa da Ana")
	texts := []string{"Bom dia, primeira mensagem", "Oi, escrevo da casa da Bia", "Respondo da casa da Ana"}
	for _, s := range []*site{s0, s1} {
		eventually(t, "the same conversation on "+s.srv.URL, func() bool {
			_, body := bia.get(s, "/chat")
			return strings.Join(order(body, texts...), "|") == strings.Join(texts, "|")
		})
	}

	// A page open on machine 1 hears what is written on machine 0.
	_, body = bia.get(s1, "/chat")
	position := between(body, `data-position="`, `"`)
	if !strings.Contains(position, "0.") || !strings.Contains(position, "1.") {
		t.Fatalf("the page on machine 1 says it has seen %q", position)
	}
	stream, stopStream := bia.listen(s1, position)
	ana.say(s0, garden)
	wantIn(t, block(t, stream), garden, "the live page on machine 1")
	stopStream()

	// Machine 0 goes away. Machine 1 goes on with what never waits, and says
	// not now to what is counted.
	secure0.stop()
	bia.say(s1, "Escrito com a outra casa fora")
	_, body = bia.get(s1, "/chat")
	wantIn(t, body, "Escrito com a outra casa fora", "machine 1 alone")
	status, body := newVisitor(t).signUp(s1, "Caio")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("a sign up with the machine that counts away: %d", status)
	}
	wantIn(t, body, "Agora não dá para criar a conta", "a sign up with the machine that counts away")

	// It comes back: each machine has what the other wrote meanwhile.
	ana.say(s0, "Voltei")
	secure0.start()
	for _, s := range []*site{s0, s1} {
		eventually(t, "machine 0 and 1 level again, on "+s.srv.URL, func() bool {
			_, body := ana.get(s, "/chat")
			return strings.Contains(body, "Escrito com a outra casa fora") && strings.Contains(body, "Voltei")
		})
	}
	if status, _ := newVisitor(t).signUp(s1, "Caio"); status != http.StatusOK {
		t.Fatalf("a sign up once the machine that counts is back: %d", status)
	}

	// Machine 1 restarts from its own folder and still knows everything.
	s1.srv.Close()
	mu.Lock()
	current1 = nil
	mu.Unlock()
	a1.Close()
	if a1, err = open(cfg1); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	current1 = a1
	mu.Unlock()
	s1 = &site{t: t, app: a1, srv: httptest.NewServer(a1.handler)}
	ana.say(s0, "Depois de a casa da Bia religar")
	eventually(t, "machine 1 following again after a restart", func() bool {
		_, body := ana.get(s1, "/chat")
		return strings.Contains(body, "Depois de a casa da Bia religar") && strings.Contains(body, "Voltei")
	})

	// The night's copy of machine 1 brings the whole machine back: its
	// number, its copy of machine 0's journal, and where machine 0 is.
	key, err := copies.LoadKey(filepath.Join(dir1, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	copyDir, restored := t.TempDir(), filepath.Join(t.TempDir(), "restored")
	if _, err := copies.Copy(dir1, copyDir, key, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := copies.Restore(copyDir, restored, key); err != nil {
		t.Fatal(err)
	}
	back, err := open(config{dataDir: restored, community: "x", short: "x", lang: "pt-BR", httpsPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	if address, ok := back.peers.Get(0); back.self() != 1 || back.name != "Vila Teste" || !ok || address != secure0.url() {
		t.Fatalf("restored as machine %d of %q, machine 0 at %q", back.self(), back.name, address)
	}
	var messages int
	back.byName["chat"].DB().QueryRow(`SELECT count(*) FROM messages`).Scan(&messages)
	if messages < 6 {
		t.Fatalf("the restored machine has %d messages", messages)
	}
}

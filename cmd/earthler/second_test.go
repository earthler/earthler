package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/replica"
)

// TestASecondMachineTakesOverTheWholeSystem is the proof of the second
// machine, with the real program on both sides: a second machine pairs with
// a running community, follows it, holds nothing it can read, and when the
// main machine is gone, a person types the copy key and the community is
// back, with its residents still signed in and a deleted text still gone.
func TestASecondMachineTakesOverTheWholeSystem(t *testing.T) {
	dir := t.TempDir()
	a, err := open(config{dataDir: dir, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	s := &site{t: t, app: a, srv: httptest.NewServer(a.handler)}

	cert, err := a.chain.Issue(time.Now(), []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewUnstartedServer(a.handler)
	secure.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	secure.StartTLS()
	welcome := httptest.NewServer(a.welcome)
	defer welcome.Close()

	ana, bia := newVisitor(t), newVisitor(t)
	ana.signUp(s, "Ana")
	bia.signUp(s, "Bia")
	ana.say(s, garden)
	bia.say(s, "Quem tem semente de feijão?")
	ana.say(s, "Esta eu vou apagar, é segredo")
	ana.remove(s, "7")
	if n, err := s.app.erase(context.Background(), time.Now()); err != nil || n != 1 {
		t.Fatalf("erase: %d, %v", n, err)
	}

	code, err := replica.NewCode(filepath.Join(dir, "replication"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "second")
	f, err := replica.Pair(context.Background(), replica.PairOptions{URL: secure.URL, PlainURL: welcome.URL,
		Code: code, Fingerprint: a.chain.Fingerprint(), Label: "casa da Bia", Dir: second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}

	// What the second machine holds is unreadable without the key.
	for _, secret := range []string{"feijão", "Ana", "BEGIN", "session"} {
		filepath.WalkDir(second, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if raw, _ := os.ReadFile(path); strings.Contains(string(raw), secret) {
					t.Errorf("%q is in the clear in %s", secret, path)
				}
			}
			return nil
		})
	}
	keyText, err := copies.LoadKey(filepath.Join(dir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkCopy(second, keyText.Text()); err != nil {
		t.Fatalf("the monthly test of the copy: %v", err)
	}
	fingerprint := a.chain.Fingerprint()

	// The main machine is gone.
	s.stop()
	secure.Close()

	taken := filepath.Join(t.TempDir(), "taken")
	if err := restore(taken, second, keyText.Text()); err != nil {
		t.Fatal(err)
	}
	s = startSite(t, taken)
	if s.app.chain.Fingerprint() != fingerprint {
		t.Fatal("the community has another certificate: every device would have to install it again")
	}
	_, body := ana.get(s, "/chat")
	wantIn(t, body, garden, "the channel on the second machine")
	wantIn(t, body, "Quem tem semente de feijão?", "the channel on the second machine")
	wantIn(t, body, "Mensagem apagada.", "the channel on the second machine")
	wantOut(t, body, "é segredo", "the channel on the second machine")
	wantIn(t, body, "Ana", "Ana's session, which the old cookie still opens")
	if err := s.app.journal.Verify(); err != nil {
		t.Fatal(err)
	}
}

// TestAMachineThatWasLostCanBeUnpaired pairs a second machine, takes its
// token away with the command, and checks that its next pull is refused.
func TestAMachineThatWasLostCanBeUnpaired(t *testing.T) {
	dir := t.TempDir()
	a, err := open(config{dataDir: dir, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	cert, err := a.chain.Issue(time.Now(), []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewUnstartedServer(a.handler)
	secure.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	secure.StartTLS()
	defer secure.Close()
	welcome := httptest.NewServer(a.welcome)
	defer welcome.Close()

	code, _ := replica.NewCode(filepath.Join(dir, "replication"), time.Now())
	f, err := replica.Pair(context.Background(), replica.PairOptions{URL: secure.URL, PlainURL: welcome.URL,
		Code: code, Fingerprint: a.chain.Fingerprint(), Label: "a machine that gets lost", Dir: filepath.Join(t.TempDir(), "second")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}

	registry, _ := replica.OpenRegistry(filepath.Join(dir, "replication"))
	machines, _ := registry.Machines()
	if len(machines) != 1 || machines[0].Label != "a machine that gets lost" {
		t.Fatalf("machines: %+v", machines)
	}
	if err := unpair(dir, "nonsense"); err == nil {
		t.Fatal("a machine that does not exist was unpaired")
	}
	if err := unpair(dir, machines[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pull(context.Background()); err != replica.ErrRevoked {
		t.Fatalf("a pull after it was unpaired: %v", err)
	}
}

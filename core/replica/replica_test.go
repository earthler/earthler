package replica

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/journal"
)

// main is a main machine in a test: its data folder, the community's
// authority, the server that hands out the copy and the two addresses a
// second machine uses, the plain one for the root and the secure one.
type main struct {
	t        *testing.T
	dataDir  string
	journal  *journal.Journal
	chain    *certificate.Chain
	key      copies.Key
	registry *Registry
	server   *Server
	secure   *httptest.Server
	plain    *httptest.Server
	repDir   string
}

func newMain(t *testing.T, entries int) *main { return newMainWith(t, entries, nil) }

// newMainWith lets a test stand between the main machine and its second
// machines, to damage or refuse what goes through.
func newMainWith(t *testing.T, entries int, wrap func(http.Handler) http.Handler) *main {
	t.Helper()
	m := &main{t: t, dataDir: t.TempDir()}
	var err error
	if m.journal, err = journal.Open(filepath.Join(m.dataDir, "journal"), journal.Options{SegmentSize: 400}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.journal.Close() })
	secrets := filepath.Join(m.dataDir, "secrets")
	if m.chain, _, err = certificate.Open(filepath.Join(secrets, "certificate"), "vila.internal", "Vila", time.Now()); err != nil {
		t.Fatal(err)
	}
	if m.key, err = copies.LoadKey(secrets); err != nil {
		t.Fatal(err)
	}
	m.add(entries)

	m.repDir = filepath.Join(m.dataDir, "replication")
	if m.registry, err = OpenRegistry(m.repDir); err != nil {
		t.Fatal(err)
	}
	m.server = &Server{Registry: m.registry, Journal: m.journal, DataDir: m.dataDir, Out: filepath.Join(m.repDir, "out"), Key: m.key}
	mux := http.NewServeMux()
	m.server.Routes(mux)

	cert, err := m.chain.Issue(time.Now(), []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatal(err)
	}
	var handler http.Handler = mux
	if wrap != nil {
		handler = wrap(mux)
	}
	m.secure = httptest.NewUnstartedServer(handler)
	m.secure.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	m.secure.StartTLS()
	t.Cleanup(m.secure.Close)
	m.plain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(m.chain.Root())
	}))
	t.Cleanup(m.plain.Close)
	return m
}

var written int

// add writes entries into the journal, each with a text that is easy to find.
func (m *main) add(n int) {
	m.t.Helper()
	for i := 0; i < n; i++ {
		written++
		if _, err := m.journal.Append([]byte(fmt.Sprintf(`{"n":%d,"text":"segredo-%d"}`, written, written))); err != nil {
			m.t.Fatal(err)
		}
	}
}

// pair does what the person does on the second machine.
func (m *main) pair(dir string) *Follower {
	m.t.Helper()
	code, err := NewCode(m.repDir, time.Now())
	if err != nil {
		m.t.Fatal(err)
	}
	f, err := Pair(context.Background(), PairOptions{URL: m.secure.URL, PlainURL: m.plain.URL, Code: code,
		Fingerprint: m.chain.Fingerprint(), Label: "second", Dir: dir})
	if err != nil {
		m.t.Fatal(err)
	}
	return f
}

// anywhere reads every file of a folder and says whether text is in one.
func anywhere(dir, text string) bool {
	found := false
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if raw, _ := os.ReadFile(path); strings.Contains(string(raw), text) {
				found = true
			}
		}
		return nil
	})
	return found
}

// TestASecondMachineFollowsAndTakesOver is the proof of this stage: a second
// machine pairs, follows the main machine's journal, reads none of it, and
// can take over with the copy key alone.
func TestASecondMachineFollowsAndTakesOver(t *testing.T) {
	m := newMain(t, 12)
	dir := filepath.Join(t.TempDir(), "second")
	f := m.pair(dir)

	first, err := f.Pull(context.Background())
	if err != nil || first.Downloaded == 0 || first.Diverged {
		t.Fatalf("first pull: %+v, %v", first, err)
	}
	if anywhere(dir, "segredo-") {
		t.Fatal("the second machine holds a text in the clear")
	}
	if anywhere(dir, "certificate") || anywhere(dir, "BEGIN") {
		t.Fatal("the second machine holds a secret in the clear")
	}
	again, err := f.Pull(context.Background())
	if err != nil || again.Downloaded != 0 {
		t.Fatalf("a pull with nothing new brought %+v, %v", again, err)
	}

	// More is written. Only what changed comes, and a machine opened again
	// from its folder continues where it was.
	m.add(30)
	f, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	later, err := f.Pull(context.Background())
	if err != nil || later.Downloaded == 0 || later.Downloaded >= later.Files {
		t.Fatalf("a pull with news: %+v, %v", later, err)
	}

	// The main machine is gone. Someone types the copy key and the second
	// machine becomes the system, with the whole journal and the same
	// secrets, so every device still trusts it.
	taken := filepath.Join(t.TempDir(), "taken")
	if _, err := copies.Restore(dir, taken, m.key); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(filepath.Join(taken, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	last, seal := j.Last()
	mainLast, mainSeal := m.journal.Last()
	if last != mainLast || seal != mainSeal || j.Verify() != nil {
		t.Fatalf("the taken over journal is at %d, the main one at %d, or it does not verify", last, mainLast)
	}
	if _, err := os.Stat(filepath.Join(taken, "secrets", "certificate", "root.pem")); err != nil {
		t.Fatalf("the certificate chain did not come: %v", err)
	}
	if _, err := copies.Check(dir, copies.Key{1}); err == nil {
		t.Fatal("the copy was read with a wrong key")
	}
}

// TestAStrangerGetsNothing tries what someone on the network could try.
func TestAStrangerGetsNothing(t *testing.T) {
	m := newMain(t, 3)
	ctx := context.Background()
	code, _ := NewCode(m.repDir, time.Now())

	// Another certificate at the address: the fingerprint does not match.
	other := newMain(t, 1)
	if _, err := Pair(ctx, PairOptions{URL: m.secure.URL, PlainURL: other.plain.URL, Code: code, Fingerprint: m.chain.Fingerprint(), Dir: t.TempDir()}); err == nil {
		t.Fatal("paired with a certificate that is not the one that was read")
	}
	// A wrong code.
	if _, err := Pair(ctx, PairOptions{URL: m.secure.URL, PlainURL: m.plain.URL, Code: "AAAA-AAAA-AAAA", Fingerprint: m.chain.Fingerprint(), Dir: t.TempDir()}); err == nil {
		t.Fatal("paired with a wrong code")
	}
	// The right code still works: the stranger's tries did not spend it.
	f := m.pair(filepath.Join(t.TempDir(), "second"))

	client := f.client
	for _, c := range []struct{ path, auth string }{
		{"/replica/index", ""},
		{"/replica/index", "Bearer nonsense"},
		{"/replica/files/manifest", ""},
	} {
		req, _ := http.NewRequest("GET", m.secure.URL+c.path, nil)
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s with %q: status %d, want 401", c.path, c.auth, res.StatusCode)
		}
	}
	for _, name := range []string{"..%2F..%2Fsecrets%2Fsession.key", "journal", "manifest.tmp", "0123456789ABCDEF0123456789abcdef", "%2e%2e", "tokens.json"} {
		req, _ := http.NewRequest("GET", m.secure.URL+"/replica/files/"+name, nil)
		req.Header.Set("Authorization", "Bearer "+f.state.Token)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMovedPermanently {
			t.Errorf("asking for %q: status %d, want 404", name, res.StatusCode)
		}
	}

	// A token that was revoked ends the following, with a clear reason.
	machines, _ := m.registry.Machines()
	if err := m.registry.Revoke(machines[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Pull(ctx); err != ErrRevoked {
		t.Fatalf("a revoked token: %v", err)
	}
}

package machines

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/replica"
	"github.com/earthler/earthler/core/scribe"
)

type fixture struct {
	dir    string
	own    *journal.Journal
	index  *index.Index
	scribe *scribe.Scribe
	server *Server
	mux    *http.ServeMux
}

func start(t *testing.T, self uint64) *fixture {
	t.Helper()
	f := &fixture{dir: t.TempDir()}
	var err error
	if f.own, err = journal.Open(filepath.Join(f.dir, "journal"), journal.Options{}); err != nil {
		t.Fatal(err)
	}
	if f.index, err = index.Open(filepath.Join(f.dir, "indexes"), Area); err != nil {
		t.Fatal(err)
	}
	if f.scribe, err = scribe.Open(scribe.Journals{Self: self, Own: f.own}, f.index); err != nil {
		t.Fatal(err)
	}
	invites, err := replica.OpenRegistry(filepath.Join(f.dir, "machines"))
	if err != nil {
		t.Fatal(err)
	}
	peers, err := OpenPeers(PeersPath(f.dir))
	if err != nil {
		t.Fatal(err)
	}
	f.server = &Server{Self: self, Label: "casa", Secrets: filepath.Join(f.dir, "secrets"), Index: f.index,
		Scribe: f.scribe, Own: f.own, Invites: invites, Peers: peers, Joined: func(uint64) error { return nil }}
	f.mux = http.NewServeMux()
	f.server.Routes(f.mux)
	t.Cleanup(func() {
		f.server.Close()
		f.scribe.Close()
		f.index.Close()
		f.own.Close()
	})
	return f
}

func (f *fixture) do(t *testing.T, method, target, token string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func added(t *testing.T, machine uint64, token string) []byte {
	t.Helper()
	c, err := event.New(event.MachineAdded, 0, 0, time.Now(), event.MachineAddedData{Machine: machine, Label: "x", Token: digest(token)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestOnlyTheMachineThatCountsAddsMachines(t *testing.T) {
	ix, err := index.Open(t.TempDir(), Area)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	entry := func(n uint64, c []byte) journal.Entry { return journal.Entry{Number: n, Content: c} }
	if err := ix.Apply(0, []journal.Entry{entry(1, added(t, 1, "um"))}); err != nil {
		t.Fatal(err)
	}
	// Machine 3 says it added machine 2: nobody believes it.
	if err := ix.Apply(3, []journal.Entry{entry(1, added(t, 2, "dois"))}); err != nil {
		t.Fatal(err)
	}
	all, err := List(context.Background(), ix)
	if err != nil || len(all) != 1 || all[0].Number != 1 {
		t.Fatalf("machines: %+v, %v", all, err)
	}
	if n, found, _ := byToken(context.Background(), ix, "dois"); found {
		t.Fatalf("a forged token belongs to machine %d", n)
	}
}

func TestTheJournalIsOnlyForTheOtherMachines(t *testing.T) {
	f := start(t, 0)
	ctx := context.Background()
	if _, err := f.scribe.Write(ctx, added(t, 0, "proprio"), added(t, 1, "outro")); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "errado", "proprio"} {
		if rec := f.do(t, "GET", "/machines/journal?after=0&live=0", token, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: %d", token, rec.Code)
		}
	}
	rec := f.do(t, "GET", "/machines/journal?after=1&live=0", "outro", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Earthler-Machine") != "0" {
		t.Fatalf("%d, machine %q", rec.Code, rec.Header().Get("Earthler-Machine"))
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("after entry 1 it sent %d lines", len(lines))
	}
	e, err := journal.ParseLine([]byte(lines[0]))
	if err != nil || e.Number != 2 {
		t.Fatalf("entry %d, %v", e.Number, err)
	}
	// Too many wrong tokens from one address and it stops listening to it.
	for range 6 {
		f.do(t, "GET", "/machines/journal?after=0", "errado", nil)
	}
	if rec := f.do(t, "GET", "/machines/journal?after=0&live=0", "outro", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after many wrong tokens: %d", rec.Code)
	}
}

func TestOnlyTheMachineThatCountsTakesMachinesIn(t *testing.T) {
	other := start(t, 1)
	if rec := other.do(t, "POST", "/machines/join", "", url.Values{"url": {"https://192.168.1.9"}}); rec.Code != http.StatusConflict {
		t.Fatalf("machine 1 took a machine in: %d", rec.Code)
	}
	f := start(t, 0)
	if rec := f.do(t, "POST", "/machines/join", "", url.Values{"url": {"http://192.168.1.9"}, "code": {"x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an address without https: %d", rec.Code)
	}
	if rec := f.do(t, "POST", "/machines/join", "", url.Values{"url": {"https://192.168.1.9"}, "code": {"AAAA-BBBB-CCCC"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("a wrong code: %d", rec.Code)
	}
	code, err := replica.NewCode(filepath.Join(f.dir, "machines"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(f.server.Secrets, 0o700)
	os.WriteFile(filepath.Join(f.server.Secrets, "session.key"), []byte("k"), 0o600)
	rec := f.do(t, "POST", "/machines/join", "", url.Values{"url": {"https://192.168.1.9"}, "code": {code}, "label": {"casa nova"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("joining: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"machine":1`) || strings.Contains(rec.Body.String(), IdentityFile) {
		t.Fatalf("the welcome: %s", rec.Body)
	}
	all, _ := List(context.Background(), f.index)
	if len(all) != 2 || all[0].Label != "casa" || all[1].Label != "casa nova" {
		t.Fatalf("machines after the first join: %+v", all)
	}
	if rec := f.do(t, "POST", "/machines/join", "", url.Values{"url": {"https://192.168.1.9"}, "code": {code}}); rec.Code != http.StatusForbidden {
		t.Fatalf("a code used twice: %d", rec.Code)
	}
}

func TestAddressesAndLines(t *testing.T) {
	for _, bad := range []string{"http://a", "https://", "https://a/path", "https://a?x=1", "https://u:p@a", "ftp://a"} {
		if CheckAddress(bad) == nil {
			t.Errorf("%q passed", bad)
		}
	}
	path := filepath.Join(t.TempDir(), "peers.json")
	p, err := OpenPeers(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := p.Set(2, "https://192.168.1.20:8443"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, _ := p.Set(2, "https://192.168.1.20:8443"); changed {
		t.Fatal("the same address changed something")
	}
	again, err := OpenPeers(path)
	if a, ok := again.Get(2); err != nil || !ok || a != "https://192.168.1.20:8443" {
		t.Fatalf("reopened: %q, %v", a, err)
	}

	rd := bufio.NewReaderSize(strings.NewReader("short\n"+strings.Repeat("x", 100)+"\n"), 16)
	if line, err := readLine(rd, 50); err != nil || string(line) != "short" {
		t.Fatalf("%q, %v", line, err)
	}
	if _, err := readLine(rd, 50); err == nil {
		t.Fatal("a line longer than the limit was read")
	}

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "secrets"), 0o700)
	os.WriteFile(filepath.Join(dir, "secrets", "session.key"), []byte("k"), 0o600)
	if _, err := Join(context.Background(), JoinOptions{DataDir: dir, Reach: "https://192.168.1.9"}); err == nil {
		t.Fatal("a folder of a community joined another")
	}
}

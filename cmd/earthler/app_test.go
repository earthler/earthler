package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// site is the server under test plus the address it answers on.
type site struct {
	t   *testing.T
	app *app
	srv *httptest.Server
}

func startSite(t *testing.T, dir string) *site {
	t.Helper()
	a, err := open(config{dataDir: dir, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	s := &site{t: t, app: a, srv: httptest.NewServer(a.handler)}
	t.Cleanup(s.stop)
	return s
}

func (s *site) stop() {
	s.srv.Close()
	s.app.Close()
}

// visitor is one browser: it keeps its own cookies.
type visitor struct {
	t      *testing.T
	client *http.Client
}

func newVisitor(t *testing.T) *visitor {
	jar, _ := cookiejar.New(nil)
	return &visitor{t: t, client: &http.Client{Jar: jar}}
}

func (v *visitor) get(s *site, path string) (int, string) {
	v.t.Helper()
	res, err := v.client.Get(s.srv.URL + path)
	if err != nil {
		v.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

// post sends a form the way a browser does from one of the site's own pages.
func (v *visitor) post(s *site, path, origin string, form url.Values) (int, string) {
	v.t.Helper()
	req, _ := http.NewRequest("POST", s.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res, err := v.client.Do(req)
	if err != nil {
		v.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func (v *visitor) signUp(s *site, name string) (int, string) {
	return v.post(s, "/sign-up", s.srv.URL, url.Values{"name": {name}})
}

func (v *visitor) say(s *site, text string) (int, string) {
	return v.post(s, "/chat", s.srv.URL, url.Values{"text": {text}})
}

func wantIn(t *testing.T, body, text, where string) {
	t.Helper()
	if !strings.Contains(body, text) {
		t.Fatalf("%s: %q is not on the page", where, text)
	}
}

func wantOut(t *testing.T, body, text, where string) {
	t.Helper()
	if strings.Contains(body, text) {
		t.Fatalf("%s: %q should not be on the page", where, text)
	}
}

const garden = "A horta precisa de água amanhã cedo"

// TestResidentWritesAndFindsText is the proof of this stage: a text written
// by one resident is found by another through the search, and it survives a
// restart and the loss of every index.
func TestResidentWritesAndFindsText(t *testing.T) {
	dir := t.TempDir()
	s := startSite(t, dir)

	ana := newVisitor(t)
	_, body := ana.get(s, "/")
	wantIn(t, body, "Criar conta", "a visitor with no account")
	wantIn(t, body, "Vila Teste", "the community name")

	if status, body := ana.signUp(s, "Ana"); status != http.StatusOK {
		t.Fatalf("sign up: status %d\n%s", status, body)
	} else {
		wantIn(t, body, "Geral", "after signing up")
		wantIn(t, body, "Ninguém escreveu ainda.", "an empty channel")
	}
	_, body = ana.say(s, garden)
	wantIn(t, body, garden, "the channel after writing")
	ana.say(s, "Reunião na sexta, depois do almoço")

	bia := newVisitor(t)
	if status, body := bia.signUp(s, "  ana "); status != http.StatusConflict {
		t.Fatalf("a taken name: status %d", status)
	} else {
		wantIn(t, body, "Já existe alguém com esse nome.", "a taken name")
	}
	bia.signUp(s, "Bia")
	_, body = bia.get(s, "/chat")
	wantIn(t, body, garden, "what another resident sees")
	wantIn(t, body, "Ana", "the author of the message")

	for _, q := range []string{"água", "agua", "AGUA", "hortas", "horta água"} {
		_, body = bia.get(s, "/search?q="+url.QueryEscape(q))
		wantIn(t, body, garden, "search for "+q)
		wantOut(t, body, "Reunião na sexta", "search for "+q)
		wantIn(t, body, "Conversa", "the origin of the result")
	}
	_, body = bia.get(s, "/search?q=chuva")
	wantIn(t, body, "Nada encontrado para essa busca.", "a search with no result")
	_, body = bia.get(s, "/search?q="+url.QueryEscape(`"OR" * NEAR(`))
	wantOut(t, body, garden, "a search made of operators")

	// The same browser is still signed in after a restart, and the text is
	// still there.
	s.stop()
	s = startSite(t, dir)
	_, body = ana.get(s, "/chat")
	wantIn(t, body, garden, "after a restart")

	// Every index is thrown away. The journal brings them back.
	s.stop()
	if err := os.RemoveAll(filepath.Join(dir, "indexes")); err != nil {
		t.Fatal(err)
	}
	s = startSite(t, dir)
	_, body = ana.get(s, "/chat")
	wantIn(t, body, garden, "after losing the indexes")
	wantIn(t, body, "Ana", "the session after losing the indexes")
	_, body = bia.get(s, "/search?q=agua")
	wantIn(t, body, garden, "search after losing the indexes")

	if err := s.app.journal.Verify(); err != nil {
		t.Fatal(err)
	}
	if last, _ := s.app.journal.Last(); last != 8 {
		t.Fatalf("the journal has %d entries, want 8: the community, the channel, two accounts with a device each, two messages", last)
	}
}

func TestOnlyResidentsGetIn(t *testing.T) {
	s := startSite(t, t.TempDir())
	stranger := newVisitor(t)
	for _, path := range []string{"/chat", "/search?q=horta"} {
		_, body := stranger.get(s, path)
		wantIn(t, body, "Criar conta", "a stranger asking for "+path)
	}
	if status, _ := stranger.say(s, "hello"); status != http.StatusOK {
		t.Fatalf("a stranger posting: status %d", status)
	}
	if last, _ := s.app.journal.Last(); last != 2 {
		t.Fatalf("a stranger's message reached the journal: %d entries", last)
	}

	// A cookie that this server did not sign is worth nothing.
	u, _ := url.Parse(s.srv.URL)
	forger := newVisitor(t)
	forger.client.Jar.SetCookies(u, []*http.Cookie{{Name: "earthler", Value: "4.0." + strings.Repeat("ab", 32)}})
	_, body := forger.get(s, "/chat")
	wantIn(t, body, "Criar conta", "a forged cookie")
}

func TestRequestsFromOtherSitesAreRefused(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)
	ana.signUp(s, "Ana")
	before, _ := s.app.journal.Last()

	for _, origin := range []string{"", "https://example.org", "http://127.0.0.1:1"} {
		if status, _ := ana.post(s, "/chat", origin, url.Values{"text": {"sent from elsewhere"}}); status != http.StatusForbidden {
			t.Errorf("origin %q: status %d, want 403", origin, status)
		}
		if status, _ := newVisitor(t).post(s, "/sign-up", origin, url.Values{"name": {"Eve"}}); status != http.StatusForbidden {
			t.Errorf("sign up with origin %q: status %d, want 403", origin, status)
		}
	}
	if after, _ := s.app.journal.Last(); after != before {
		t.Fatalf("a refused request wrote %d entries", after-before)
	}
}

func TestBadInputIsRefusedKindly(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)
	if status, body := ana.signUp(s, "   "); status != http.StatusBadRequest {
		t.Fatalf("an empty name: status %d", status)
	} else {
		wantIn(t, body, "Escreva um nome para continuar.", "an empty name")
	}
	if status, _ := ana.signUp(s, strings.Repeat("a", 41)); status != http.StatusBadRequest {
		t.Fatalf("a long name: status %d", status)
	}
	ana.signUp(s, "Ana")
	if status, body := ana.say(s, "  \n "); status != http.StatusBadRequest {
		t.Fatalf("an empty message: status %d", status)
	} else {
		wantIn(t, body, "Escreva uma mensagem antes de enviar.", "an empty message")
	}
	if status, _ := ana.say(s, strings.Repeat("á", 4001)); status != http.StatusBadRequest {
		t.Fatalf("a long message: status %d", status)
	}

	// Text is shown as text, never as part of the page.
	_, body := ana.say(s, `<script>alert("x")</script>`)
	wantOut(t, body, `<script>alert`, "a message with a script")
	wantIn(t, body, `&lt;script&gt;`, "a message with a script")
}

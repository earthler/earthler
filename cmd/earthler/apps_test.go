package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestThePortalAndTheFourApps is the proof of what a resident sees: the
// community's address opens the portal, with the search in the middle and
// the four apps around it, each on a path of its own with the manifest that
// makes it an icon. The parts not built yet say so, inside their app.
func TestThePortalAndTheFourApps(t *testing.T) {
	s := startSite(t, t.TempDir())
	ana := newVisitor(t)

	// Nobody signed in: the portal and every app send to the sign up first.
	for _, path := range []string{"/", "/talk/", "/learn/", "/decide/", "/ask/", "/talk/calls", "/decide/map"} {
		_, body := ana.get(s, path)
		wantIn(t, body, "Criar conta", path+" without an account")
	}

	_, body := ana.signUp(s, "Ana")
	wantIn(t, body, `action="/search"`, "the search in the portal")
	for _, app := range []string{"talk", "learn", "decide", "ask"} {
		wantIn(t, body, `href="/`+app+`/"`, "the portal opens "+app)
		wantIn(t, body, `href="/`+app+`/?add=1"`, "the portal's button to add "+app)
	}
	wantOut(t, body, `rel="manifest"`, "the portal, which never becomes an icon")

	names := map[string]string{"talk": "Talk", "learn": "Learn", "decide": "Decide", "ask": "Ask"}
	for app, name := range names {
		status, body := ana.get(s, "/"+app+"/")
		if status != http.StatusOK {
			t.Fatalf("/%s/: status %d", app, status)
		}
		wantIn(t, body, `<link rel="manifest" href="/`+app+`/manifest.webmanifest">`, "the manifest of "+app)
		wantIn(t, body, `href="/brand/apps/`+app+`-180.png"`, "the icon of "+app+" on an iPhone")
		wantIn(t, body, `class="abas"`, "the bar of "+app)
		wantIn(t, body, `href="/"`, "the way from "+app+" to the portal")

		res, err := http.Get(s.srv.URL + "/" + app + "/manifest.webmanifest")
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Name, StartURL, Scope, Display string
			Icons                          []struct{ Src, Sizes string }
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		json.Unmarshal([]byte(strings.NewReplacer(`"start_url"`, `"StartURL"`).Replace(string(raw))), &m)
		if m.Name != name || m.StartURL != "/"+app+"/" || m.Scope != "/"+app+"/" || m.Display != "standalone" || len(m.Icons) != 2 {
			t.Fatalf("the manifest of %s: %s", app, raw)
		}
		for _, icon := range m.Icons {
			res, err := http.Get(s.srv.URL + icon.Src)
			if err != nil || res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("the icon %s: %v, %v", icon.Src, err, res)
			}
			res.Body.Close()
		}
	}

	// A part that is not built yet says so, in its app, with its bar.
	_, body = ana.get(s, "/talk/calls")
	wantIn(t, body, "ainda não existe nesta versão", "a part not built yet")
	wantIn(t, body, `aria-current="page"`, "the bar marks where the resident is")

	// Talk's home lists the open channel, and the old address of the
	// conversation still leads to it.
	ana.say(s, garden)
	_, body = ana.get(s, "/talk/")
	wantIn(t, body, `href="/talk/channel/2"`, "Talk's home")
	wantIn(t, body, "Ana: "+garden, "the last message under the channel")
	_, body = ana.get(s, "/chat")
	wantIn(t, body, garden, "the old address of the conversation")
	if status, _ := ana.get(s, "/talk/channel/999"); status != http.StatusNotFound {
		t.Fatalf("a channel that does not exist: %d", status)
	}

	// Learn searches without leaving Learn.
	_, body = ana.get(s, "/learn/search?q=horta")
	wantIn(t, body, garden, "a search from Learn")
	wantIn(t, body, `data-app="learn"`, "a search from Learn stays in Learn")
	wantIn(t, body, `action="/learn/search"`, "the next search from Learn")

	// The mark is on the welcome page too, and no folder is listed.
	welcome := httptest.NewServer(s.app.welcome)
	defer welcome.Close()
	for path, want := range map[string]int{"/brand/earthler-mark.png": http.StatusOK, "/brand/": http.StatusNotFound, "/brand/apps/": http.StatusNotFound} {
		res, err := http.Get(welcome.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("%s on the welcome page: %d, want %d", path, res.StatusCode, want)
		}
	}
}

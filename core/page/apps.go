package page

import (
	"encoding/json"
	"html/template"
	"net/http"
)

// App is one of the four apps a resident adds to the phone. Each lives on
// a path of its own on the community's one address, and no path sits inside
// another, so a phone installs each one apart. Its name, description and
// the labels of its bar are in the locales, under app.<key>.
type App struct {
	Key  string // talk, learn, decide or ask: the path and the locale key
	Tabs []Tab  // the bar at the bottom, the first one being the app's home
}

// Tab is one item of an app's bar.
type Tab struct {
	Key  string // the locale key, under app.<app>.tab.<key>
	Path string // where it leads
	Icon string // the line icon that draws it
}

// Apps are the four apps, in the order the portal shows them.
var Apps = []App{
	{Key: "talk", Tabs: []Tab{
		{"conversations", "/talk/", "i-talk"}, {"calls", "/talk/calls", "i-fone"}, {"alerts", "/talk/alerts", "i-sino"}}},
	{Key: "learn", Tabs: []Tab{
		{"subjects", "/learn/", "i-grade"}, {"lessons", "/learn/lessons", "i-aula"}, {"wiki", "/learn/wiki", "i-wiki"},
		{"collection", "/learn/collection", "i-acervo"}, {"cinema", "/learn/cinema", "i-cine"}}},
	{Key: "decide", Tabs: []Tab{
		{"decisions", "/decide/", "i-voto"}, {"map", "/decide/map", "i-decide"}, {"votes", "/decide/votes", "i-gente"}}},
	{Key: "ask", Tabs: []Tab{
		{"ask", "/ask/", "i-ask"}, {"answers", "/ask/answers", "i-lista"}, {"missing", "/ask/missing", "i-falta"}}},
}

// app returns the app with this key.
func app(key string) (App, bool) {
	for _, a := range Apps {
		if a.Key == key {
			return a, true
		}
	}
	return App{}, false
}

// Who says the name and the role of the person behind a request that a
// signed-in wrapper let through.
type Who func(*http.Request) (name, role string)

// AppRoutes serves, for each app, the manifest a phone reads when the app is
// added to its home screen, and a page that says it is not built yet for
// each tab of its bar that no screen serves. built lists the paths that a
// screen serves.
func (r *Renderer) AppRoutes(mux *http.ServeMux, signedIn func(http.Handler) http.Handler, who Who, built ...string) {
	served := map[string]bool{}
	for _, p := range built {
		served[p] = true
	}
	for _, a := range Apps {
		mux.HandleFunc("GET /"+a.Key+"/manifest.webmanifest", r.manifest(a))
		for _, t := range a.Tabs {
			if !served[t.Path] {
				mux.Handle("GET "+t.Path, signedIn(r.soon(a, t, who)))
			}
		}
	}
}

// manifest says to a phone what the app is called, where it starts and
// where it ends, and which icon it gets. The scope is the app's own path,
// so the four are four apps and the portal is none of them.
func (r *Renderer) manifest(a App) http.HandlerFunc {
	type icon struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose,omitempty"`
	}
	body, err := json.Marshal(map[string]any{
		"name":             r.T("app." + a.Key + ".name"),
		"short_name":       r.T("app." + a.Key + ".name"),
		"description":      r.T("app." + a.Key + ".description"),
		"lang":             r.lang,
		"id":               "/" + a.Key + "/",
		"start_url":        "/" + a.Key + "/",
		"scope":            "/" + a.Key + "/",
		"display":          "standalone",
		"background_color": "#F3EFE7",
		"theme_color":      "#F3EFE7",
		"icons": []icon{
			{Src: "/brand/apps/" + a.Key + "-192.png", Sizes: "192x192", Type: "image/png"},
			{Src: "/brand/apps/" + a.Key + "-512.png", Sizes: "512x512", Type: "image/png"},
		},
	})
	if err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(body)
	}
}

// soon is the page of a part of an app that is not built yet. It says so in
// the same face as every other page, inside the app, with its bar.
func (r *Renderer) soon(a App, t Tab, who Who) http.Handler {
	tmpl := template.Must(template.Must(r.base.Clone()).ParseFS(files, "soon.html"))
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name, role := who(req)
		part := r.T("app." + a.Key + ".tab." + t.Key)
		r.Render(w, http.StatusOK, tmpl, Data{Title: part, App: a.Key, Tab: t.Key, User: name, Role: role, Body: soonBody{Part: part, Icon: t.Icon}})
	})
}

type soonBody struct{ Part, Icon string }

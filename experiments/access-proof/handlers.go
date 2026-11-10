package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// probeImage is a transparent 1x1 GIF. The welcome page loads it over HTTPS
// to find out whether the device already trusts the certificate.
var probeImage = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")

// scenarios are the situations a person can record from the test page.
var scenarios = map[string]bool{
	"internet-on":           true,
	"internet-cut-data-off": true,
	"internet-cut-data-on":  true,
}

type forgedLink struct {
	Name string
	URL  string
	Note string
}

type pageData struct {
	Secure     bool
	Domain     string
	ShortPrint string
	FullPrint  string
	Device     string // iphone, android or other
	Host       string
	SecureURL  string // the secure site, at the address the visitor used
	ControlURL string // the same test page without HTTPS
	HomeURL    string // the secure site, by address
	Forged     []forgedLink
}

func (s *server) handler(secure bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if secure {
			s.render(w, r, "test.html", secure)
			return
		}
		s.render(w, r, "welcome.html", secure)
	})
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, "test.html", secure)
	})
	mux.HandleFunc("GET /ca.crt", s.rootCertificate)
	mux.HandleFunc("GET /probe.gif", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		w.Write(probeImage)
	})
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, s.static, "sw.js")
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(s.static)))
	mux.HandleFunc("POST /api/result", func(w http.ResponseWriter, r *http.Request) {
		s.saveResult(w, r, secure)
	})
	if !secure {
		return noStore(mux)
	}
	return noStore(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isForged(hostOnly(r.Host)) {
			s.render(w, r, "forged.html", true)
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

func (s *server) render(w http.ResponseWriter, r *http.Request, page string, secure bool) {
	host := hostOnly(r.Host)
	print := s.chain.fingerprint()
	data := pageData{
		Secure:     secure,
		Domain:     s.chain.domain,
		ShortPrint: grouped(print[:8]),
		FullPrint:  grouped(print),
		Device:     device(r.UserAgent()),
		Host:       host,
		SecureURL:  baseURL("https", host, s.opts.httpsPort),
		ControlURL: baseURL("http", host, s.opts.httpPort) + "test",
		HomeURL:    baseURL("https", s.ips[0].String(), s.opts.httpsPort),
	}
	if secure && page == "test.html" {
		data.Forged = s.forgedLinks(host)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pages.ExecuteTemplate(w, page, data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

// forgedLinks lists names that reach this machine but that the community
// certificate may not cover.
func (s *server) forgedLinks(host string) []forgedLink {
	var links []forgedLink
	if s.mdns != "" {
		links = append(links, forgedLink{
			Name: s.mdns,
			URL:  baseURL("https", s.mdns, s.opts.httpsPort),
			Note: "Works with no internet. Some Android versions cannot find names that end in .local.",
		})
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		ip = s.ips[0].To4()
	}
	if ip != nil {
		name := strings.ReplaceAll(ip.String(), ".", "-") + ".sslip.io"
		links = append(links, forgedLink{
			Name: name,
			URL:  baseURL("https", name, s.opts.httpsPort),
			Note: "Needs internet. Uses the public service sslip.io to point a public name at this machine.",
		})
	}
	return links
}

// rootCertificate sends the certificate that devices install. An iPhone only
// offers to install it as a profile when it is not sent as a download.
func (s *server) rootCertificate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	if device(r.UserAgent()) != "iphone" {
		w.Header().Set("Content-Disposition", `attachment; filename="earthler-access-proof.crt"`)
	}
	w.Write(s.chain.root.Raw)
}

func (s *server) saveResult(w http.ResponseWriter, r *http.Request, secure bool) {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			http.Error(w, "wrong origin", http.StatusForbidden)
			return
		}
	}
	var in struct {
		Scenario string            `json:"scenario"`
		Opened   string            `json:"opened"` // how the page opened: no-warning or through-warning
		Notes    string            `json:"notes"`
		Checks   map[string]string `json:"checks"`
	}
	body := http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(body).Decode(&in); err != nil || !scenarios[in.Scenario] || len(in.Checks) > 24 {
		http.Error(w, "invalid result", http.StatusBadRequest)
		return
	}
	if secure && in.Opened != "no-warning" && in.Opened != "through-warning" {
		http.Error(w, "say how the page opened", http.StatusBadRequest)
		return
	}
	res := result{
		Remote:    remoteIP(r.RemoteAddr),
		Scheme:    "http",
		Host:      hostOnly(r.Host),
		Scenario:  in.Scenario,
		Notes:     clip(in.Notes, 300),
		UserAgent: clip(r.UserAgent(), 300),
		Checks:    map[string]string{},
	}
	if secure {
		res.Scheme = "https"
	}
	for k, v := range in.Checks {
		res.Checks[clip(k, 40)] = clip(v, 120)
	}
	if secure {
		res.Checks["opened"] = in.Opened
	}
	if err := s.results.add(res); err != nil {
		log.Printf("save result: %v", err)
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func device(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "iPhone"), strings.Contains(userAgent, "iPad"), strings.Contains(userAgent, "iPod"):
		return "iphone"
	case strings.Contains(userAgent, "Android"):
		return "android"
	}
	return "other"
}

// clip shortens s to at most n characters.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Package welcome is the only screen served without HTTPS. It introduces the
// community to a new device and hands over the community certificate. No
// resident data ever passes through it.
package welcome

import (
	"embed"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/page"
)

//go:embed welcome.html welcome.js
var files embed.FS

// probeImage is a transparent 1x1 GIF. The welcome page loads it over HTTPS
// to find out whether the device already trusts the certificate.
var probeImage = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")

// Screen serves the welcome page and the certificate.
type Screen struct {
	chain     *certificate.Chain
	pages     *page.Renderer
	tmpl      *template.Template
	httpsPort int
}

// New returns the screen. httpsPort is where the secure site answers.
func New(chain *certificate.Chain, pages *page.Renderer, httpsPort int) *Screen {
	return &Screen{chain: chain, pages: pages, tmpl: pages.Screen(files, "welcome.html"), httpsPort: httpsPort}
}

// Routes adds the screen to the plain HTTP server.
func (s *Screen) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.show)
	mux.HandleFunc("GET /certificate.crt", s.certificate)
	mux.HandleFunc("GET /welcome.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, files, "welcome.js")
	})
}

// Probe answers on the secure server, for the welcome page to test trust.
func Probe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(probeImage)
}

type body struct {
	Domain    string
	Print     string // first characters of the fingerprint, to compare with the poster
	FullPrint string
	Device    string // iphone, android or other
	SecureURL string
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request) {
	secure := s.secureURL(r.Host)
	print := s.chain.Fingerprint()
	s.pages.Render(w, http.StatusOK, s.tmpl, page.Data{
		Title:  s.pages.T("welcome.title"),
		Script: "/welcome.js",
		Images: strings.TrimSuffix(secure, "/"),
		Body: body{
			Domain:    s.chain.Domain(),
			Print:     pairs(print[:8]),
			FullPrint: pairs(print),
			Device:    device(r.UserAgent()),
			SecureURL: secure,
		},
	})
}

// certificate sends the root that devices install. An iPhone only offers to
// install it as a profile when it is not sent as a download.
func (s *Screen) certificate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	if device(r.UserAgent()) != "iphone" {
		w.Header().Set("Content-Disposition", `attachment; filename="earthler-community.crt"`)
	}
	w.Write(s.chain.Root())
}

// secureURL is the secure site at the same address the visitor used.
func (s *Screen) secureURL(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if s.httpsPort == 443 {
		return "https://" + host + "/"
	}
	return "https://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(s.httpsPort)) + "/"
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

// pairs writes hex characters two by two, the way phones show fingerprints.
func pairs(hex string) string {
	var b strings.Builder
	for i := 0; i < len(hex); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String()
}

// Package welcome is the only screen served without HTTPS. It introduces the
// community to a new device and walks the person through installing the
// community certificate, one page per kind of device. No resident data ever
// passes through it.
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

//go:embed *.html welcome.js
var files embed.FS

// probeImage is a transparent 1x1 GIF. The pages load it over HTTPS to find
// out whether the device already trusts the certificate.
var probeImage = []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")

// devices are the kinds of device that have a page of their own.
var devices = []string{"iphone", "android", "windows", "mac", "linux", "computer"}

// Screen serves the welcome page, the certificate pages and the certificate.
type Screen struct {
	chain     *certificate.Chain
	pages     *page.Renderer
	tmpl      map[string]*template.Template
	httpsPort int
}

// New returns the screen. httpsPort is where the secure site answers.
func New(chain *certificate.Chain, pages *page.Renderer, httpsPort int) *Screen {
	s := &Screen{chain: chain, pages: pages, tmpl: map[string]*template.Template{}, httpsPort: httpsPort}
	for _, name := range []string{"welcome", "steps", "safari"} {
		s.tmpl[name] = pages.Screen(files, name+".html", "shared.html")
	}
	return s
}

// Routes adds the screen to the plain HTTP server.
func (s *Screen) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.show)
	for _, d := range devices {
		mux.HandleFunc("GET /certificate/"+d, s.steps(d))
	}
	mux.HandleFunc("GET /certificate.crt", s.certificate)
	mux.HandleFunc("GET /certificate.mobileconfig", s.profile)
	mux.HandleFunc("GET /welcome.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, files, "welcome.js")
	})
}

// Probe answers on the secure server, for the pages to test trust.
func Probe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(probeImage)
}

// step is one card of a certificate page.
type step struct {
	Number   int
	Title    string
	Text     string
	Note     string // a warning under the text, or empty
	Download bool   // this step has the download button
	Confirm  string // label of the "I did this" button, or empty
}

type body struct {
	Device    string   // the kind of device this page is for
	Devices   []string // every kind that has a page, for the links
	Guess     string   // the kind of device the browser seems to be
	Domain    string
	RootName  string // what the device shows for the root once installed
	Print     string // first characters of the fingerprint, to compare with the poster
	FullPrint string
	Heading   string
	Scope     string // what the certificate covers, and what it cannot cover
	Steps     []step
	Late      string // what to check when trust does not arrive
	Download  string // the file the download button points to
	SecureURL string
	PlainURL  string // this site without HTTPS, for the person to type into Safari
}

func (s *Screen) show(w http.ResponseWriter, r *http.Request) {
	b := s.body(r, "")
	b.Guess = device(r.UserAgent())
	s.render(w, "welcome", s.pages.T("welcome.title"), b)
}

// steps serves the page of one kind of device. An iPhone that is not using
// Safari gets the page that sends it there, because only Safari can hand the
// profile to the Settings app.
func (s *Screen) steps(d string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b := s.body(r, d)
		if d == "iphone" && device(r.UserAgent()) == "iphone" && !safari(r.UserAgent()) {
			s.render(w, "safari", s.pages.T("certificate.iphone.safari.title"), b)
			return
		}
		s.render(w, "steps", s.pages.T("certificate."+d+".title"), b)
	}
}

// body gathers what every page needs. The texts of the steps come from the
// language file, with the community's own names filled in.
func (s *Screen) body(r *http.Request, d string) body {
	print := s.chain.Fingerprint()
	b := body{
		Device:    d,
		Devices:   devices,
		Domain:    s.chain.Domain(),
		RootName:  s.chain.RootName(),
		Print:     pairs(print[:8]),
		FullPrint: pairs(print),
		SecureURL: s.secureURL(r.Host),
		PlainURL:  "http://" + r.Host + "/",
		Download:  "/certificate.crt",
	}
	if d == "" {
		return b
	}
	if d == "iphone" {
		b.Download = "/certificate.mobileconfig"
	}
	fill := strings.NewReplacer("{domain}", b.Domain, "{root}", b.RootName, "{community}", s.pages.Community())
	t := func(key string) string { return fill.Replace(s.pages.T("certificate." + d + "." + key)) }
	b.Heading = t("heading")
	b.Late = t("late")
	b.Scope = fill.Replace(s.pages.T("certificate.scope"))
	for i := 1; ; i++ {
		n := strconv.Itoa(i)
		title := s.pages.T("certificate." + d + "." + n + ".title")
		if title == "certificate."+d+"."+n+".title" {
			break // no more steps in the language file
		}
		st := step{Number: i, Title: title, Text: t(n + ".text"), Download: i == 1}
		if key := "certificate." + d + "." + n + ".note"; s.pages.T(key) != key {
			st.Note = fill.Replace(s.pages.T(key))
		}
		if key := "certificate." + d + "." + n + ".confirm"; s.pages.T(key) != key {
			st.Confirm = s.pages.T(key)
		}
		b.Steps = append(b.Steps, st)
	}
	return b
}

func (s *Screen) render(w http.ResponseWriter, name, title string, b body) {
	s.pages.Render(w, http.StatusOK, s.tmpl[name], page.Data{
		Title:  title,
		Script: "/welcome.js",
		Images: strings.TrimSuffix(b.SecureURL, "/"),
		Body:   b,
	})
}

// certificate sends the root as a file, for Android and computers.
func (s *Screen) certificate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="`+s.chain.Domain()+`.crt"`)
	w.Write(s.chain.Root())
}

// profile sends the root inside a configuration profile, which is how an
// iPhone expects to receive a certificate. Sent as a download, Safari would
// save it as a file instead of offering to install it.
func (s *Screen) profile(w http.ResponseWriter, r *http.Request) {
	fill := strings.NewReplacer("{domain}", s.chain.Domain(), "{community}", s.pages.Community())
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Write(s.chain.Profile(certificate.ProfileText{
		Name:         fill.Replace(s.pages.T("certificate.profile.name")),
		Organization: fill.Replace(s.pages.T("certificate.profile.organization")),
		Description:  fill.Replace(s.pages.T("certificate.profile.description")),
	}))
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

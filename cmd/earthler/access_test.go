package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestDeviceWithTheCertificateGetsIn is the proof of the access stage as far
// as a program can take it: a device that installed the community root opens
// the system over HTTPS with no complaint, and a device that did not is kept
// out. What real phones do is the job of the access proof experiment.
func TestDeviceWithTheCertificateGetsIn(t *testing.T) {
	dir := t.TempDir()
	cfg := config{dataDir: dir, community: "Vila Teste", short: "vila", lang: "pt-BR", httpsPort: 443}
	a, err := open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { a.Close() }()

	cert, err := a.chain.Issue(time.Now(), []net.IP{net.ParseIP("192.168.1.50")})
	if err != nil {
		t.Fatal(err)
	}
	secure := httptest.NewUnstartedServer(a.handler)
	secure.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	secure.StartTLS()
	defer secure.Close()

	root, err := x509.ParseCertificate(a.chain.Root())
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	jar, _ := cookiejar.New(nil)
	trusting := &http.Client{
		Jar:           jar,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "vila.internal"}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	res, err := trusting.Get(secure.URL + "/probe.gif")
	if err != nil {
		t.Fatalf("a device with the root could not connect: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/gif" {
		t.Fatalf("probe: status %d, type %s", res.StatusCode, res.Header.Get("Content-Type"))
	}

	// Over HTTPS the session cookie is marked so it never travels in the clear.
	req, _ := http.NewRequest("POST", secure.URL+"/sign-up", strings.NewReader(url.Values{"name": {"Ana"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", secure.URL)
	res, err = trusting.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cookie := res.Header.Get("Set-Cookie")
	for _, want := range []string{"Secure", "HttpOnly", "SameSite=Strict"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("the session cookie is missing %s: %s", want, cookie)
		}
	}

	stranger := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: "vila.internal"}}}
	if res, err := stranger.Get(secure.URL + "/probe.gif"); err == nil {
		res.Body.Close()
		t.Fatal("a device without the root connected with no complaint")
	}

	// The plain HTTP side: the welcome page and the certificate, nothing else.
	plain := httptest.NewServer(a.welcome)
	defer plain.Close()
	get := func(path, agent string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", plain.URL+path, nil)
		req.Header.Set("User-Agent", agent)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res, string(body)
	}
	res, body := get("/", "Mozilla/5.0 (Linux; Android 14)")
	print := a.chain.Fingerprint()
	for _, want := range []string{"Boas-vindas", "vila.internal", print[0:2] + " " + print[2:4], `href="https://127.0.0.1/"`, `<details open>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the welcome page is missing %q", want)
		}
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "img-src 'self' https://127.0.0.1") {
		t.Errorf("the welcome page may not load its probe: %s", csp)
	}
	res, body = get("/certificate.crt", "Mozilla/5.0 (Linux; Android 14)")
	if !bytes.Equal([]byte(body), a.chain.Root()) || res.Header.Get("Content-Type") != "application/x-x509-ca-cert" {
		t.Error("the certificate download is not the community root")
	}
	if res.Header.Get("Content-Disposition") == "" {
		t.Error("Android should get the certificate as a download")
	}
	if res, _ = get("/certificate.crt", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)"); res.Header.Get("Content-Disposition") != "" {
		t.Error("an iPhone should get the certificate as a profile, not as a download")
	}
	for _, path := range []string{"/chat", "/search?q=x", "/sign-up"} {
		if res, _ := get(path, "any"); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s answered %d over plain HTTP, want 404", path, res.StatusCode)
		}
	}

	// A restart must keep the same root, or every device would be locked out.
	a.Close()
	if a, err = open(cfg); err != nil {
		t.Fatal(err)
	}
	if a.chain.Fingerprint() != print {
		t.Fatal("the root changed after a restart")
	}
}

func TestBadShortNameIsRefused(t *testing.T) {
	for _, short := range []string{"", "Vila", "vila nova", "-vila", "vila.internal"} {
		a, err := open(config{dataDir: t.TempDir(), community: "Vila", short: short, lang: "pt-BR", httpsPort: 443})
		if err == nil {
			a.Close()
			t.Errorf("short name %q was accepted", short)
		}
	}
}

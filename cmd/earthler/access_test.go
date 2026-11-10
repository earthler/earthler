package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
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
	android := "Mozilla/5.0 (Linux; Android 14) Chrome/130.0 Mobile Safari/537.36"
	safari := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/27.0.1 Mobile/15E148 Safari/604.1"
	chrome := "Mozilla/5.0 (iPhone; CPU iPhone OS 27_0_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/156.0.8078.54 Mobile/15E148 Safari/604.1"
	windows := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	mac := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15"
	linux := "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0"
	print := a.chain.Fingerprint()
	pages := []struct {
		path, agent string
		want        []string
	}{
		{"/", android, []string{"Boas-vindas", "Parece um Android.", `href="/certificate/android"`, `href="https://127.0.0.1/"`}},
		{"/certificate/android", android, []string{"vila.internal", print[0:2] + " " + print[2:4], "Instalar assim mesmo", `href="/certificate.crt"`, `data-step="4"`}},
		{"/certificate/iphone", safari, []string{"Certificados Confiáveis", "Vila Teste (Earthler)", `href="/certificate.mobileconfig"`, "8 minutos"}},
		{"/certificate/iphone", chrome, []string{"Abra esta página no Safari", "http://127.0.0.1"}},
		{"/certificate/windows", windows, []string{"Autoridades de Certificação Raiz Confiáveis", `href="/certificate.crt"`, `href="/certificate/mac"`}},
		{"/certificate/mac", mac, []string{"Acesso às Chaves", "Confiar Sempre", "Vila Teste (Earthler)"}},
		{"/certificate/linux", linux, []string{"Gerenciar certificados", "Autoridades", `data-step="4"`}},
		{"/certificate/computer", "curl/8.7.1", []string{`href="/certificate/windows"`, `href="/certificate/mac"`, `href="/certificate/linux"`, "Não reconhecemos"}},
	}
	for _, pg := range pages {
		res, body := get(pg.path, pg.agent)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s answered %d", pg.path, res.StatusCode)
		}
		for _, want := range pg.want {
			if !strings.Contains(body, want) {
				t.Errorf("%s is missing %q", pg.path, want)
			}
		}
		if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "img-src 'self' https://127.0.0.1") {
			t.Errorf("%s may not load its probe: %s", pg.path, csp)
		}
	}
	res, body := get("/certificate.crt", android)
	if !bytes.Equal([]byte(body), a.chain.Root()) || res.Header.Get("Content-Type") != "application/x-x509-ca-cert" {
		t.Error("the certificate download is not the community root")
	}
	if res.Header.Get("Content-Disposition") == "" {
		t.Error("the certificate file should arrive as a download")
	}
	res, body = get("/certificate.mobileconfig", safari)
	if res.Header.Get("Content-Type") != "application/x-apple-aspen-config" || res.Header.Get("Content-Disposition") != "" {
		t.Errorf("the profile must arrive as a profile, not a download: %s %s", res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"))
	}
	for _, want := range []string{base64.StdEncoding.EncodeToString(a.chain.Root()), "Vila Teste", "Certificados Confiáveis"} {
		if !strings.Contains(body, want) {
			t.Errorf("the profile is missing %q", want)
		}
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

package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

var testTime = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func newTestChain(t *testing.T) *chain {
	t.Helper()
	c, err := newChain("demo.internal", testTime)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func issueFor(t *testing.T, c *chain, names []string, ips ...string) *tls.Certificate {
	t.Helper()
	var parsed []net.IP
	for _, ip := range ips {
		parsed = append(parsed, net.ParseIP(ip))
	}
	cert, err := c.issue(testTime, names, parsed)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// verify checks a certificate the way a device that installed the root would.
func verify(c *chain, leaf *x509.Certificate, name string, extra ...*x509.Certificate) error {
	roots := x509.NewCertPool()
	roots.AddCert(c.root)
	inter := x509.NewCertPool()
	inter.AddCert(c.issuer)
	for _, e := range extra {
		inter.AddCert(e)
	}
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		DNSName:       name,
		CurrentTime:   testTime,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func wantReason(t *testing.T, err error, want x509.InvalidReason) {
	t.Helper()
	var invalid x509.CertificateInvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a certificate error with reason %d, got %v", want, err)
	}
	if invalid.Reason != want {
		t.Fatalf("want reason %d, got %d (%v)", want, invalid.Reason, err)
	}
}

func TestServerCertificateIsAccepted(t *testing.T) {
	c := newTestChain(t)
	cert := issueFor(t, c, []string{"demo.internal", "wiki.demo.internal"}, "192.168.0.173", "10.1.2.3")
	for _, name := range []string{"demo.internal", "wiki.demo.internal", "192.168.0.173", "10.1.2.3"} {
		if err := verify(c, cert.Leaf, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestForgedNameIsRejected(t *testing.T) {
	c := newTestChain(t)
	names := []string{"www.example.org", "notdemo.internal", "demo.internal.example.org", "machine.local"}
	for _, name := range names {
		cert := issueFor(t, c, []string{name})
		wantReason(t, verify(c, cert.Leaf, name), x509.CANotAuthorizedForThisName)
	}
}

func TestPublicAddressIsRejected(t *testing.T) {
	c := newTestChain(t)
	cert := issueFor(t, c, nil, "8.8.8.8")
	wantReason(t, verify(c, cert.Leaf, "8.8.8.8"), x509.CANotAuthorizedForThisName)
}

func TestIssuerCannotCreateAnotherIssuer(t *testing.T) {
	c := newTestChain(t)
	second, err := newChain("demo.internal", testTime) // only used as a source of a key and a template
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := caTemplate("second issuer", "demo.internal", testTime)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.issuer, &second.issuerKey.PublicKey, c.issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	below := &chain{domain: c.domain, root: c.root, issuer: sub, issuerKey: second.issuerKey}
	cert := issueFor(t, below, []string{"demo.internal"})
	wantReason(t, verify(c, cert.Leaf, "demo.internal", sub), x509.TooManyIntermediates)
}

func TestValidityPeriods(t *testing.T) {
	c := newTestChain(t)
	for _, ca := range []*x509.Certificate{c.root, c.issuer} {
		if want := testTime.AddDate(caYears, 0, 0); !ca.NotAfter.Equal(want) {
			t.Errorf("%s ends %s, want %s", ca.Subject.CommonName, ca.NotAfter, want)
		}
	}
	cert := issueFor(t, c, []string{"demo.internal"})
	if life := cert.Leaf.NotAfter.Sub(testTime); life > 398*24*time.Hour {
		t.Errorf("server certificate lives %s, more than 398 days", life)
	}
	if got := cert.Leaf.Subject; got.CommonName == (pkix.Name{}).CommonName {
		t.Error("server certificate has no subject")
	}
}

func TestOnlyTheIssuerKeyIsSaved(t *testing.T) {
	c := newTestChain(t)
	dir := filepath.Join(t.TempDir(), "data")
	if err := c.save(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if want := []string{issuerKeyFile, issuerFile, rootFile}; !equal(names, want) {
		t.Fatalf("saved %v, want %v", names, want)
	}
	info, err := os.Stat(filepath.Join(dir, issuerKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("issuer key mode is %o, want 600", info.Mode().Perm())
	}

	loaded, err := loadChain(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.issuerKey.PublicKey.Equal(c.issuer.PublicKey) {
		t.Error("the saved key is not the issuer key")
	}
	if loaded.issuerKey.PublicKey.Equal(c.root.PublicKey) {
		t.Error("the saved key is the root key")
	}
	if loaded.domain != "demo.internal" || loaded.fingerprint() != c.fingerprint() {
		t.Error("the loaded chain differs from the saved one")
	}
	cert := issueFor(t, loaded, []string{"demo.internal"})
	if err := verify(c, cert.Leaf, "demo.internal"); err != nil {
		t.Errorf("certificate from the loaded chain: %v", err)
	}
}

func TestMissingChainIsReportedAsNotExist(t *testing.T) {
	_, err := loadChain(filepath.Join(t.TempDir(), "nothing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want os.ErrNotExist, got %v", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

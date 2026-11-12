package certificate

import (
	"crypto/rand"
	"crypto/x509"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// verify checks a certificate the way a device that installed the root would.
func verify(c *Chain, leaf *x509.Certificate, name string) error {
	roots, inter := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(c.root)
	inter.AddCert(c.issuer)
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: inter, DNSName: name, CurrentTime: testTime,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func open(t *testing.T, dir string) (*Chain, bool) {
	t.Helper()
	c, created, err := Open(dir, "vila.internal", "Vila Teste", testTime)
	if err != nil {
		t.Fatal(err)
	}
	return c, created
}

func TestServerCertificateIsAccepted(t *testing.T) {
	c, created := open(t, t.TempDir())
	if !created {
		t.Fatal("a new chain was not reported as created")
	}
	cert, err := c.Issue(testTime, []net.IP{net.ParseIP("192.168.1.50"), net.ParseIP("10.1.2.3")})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vila.internal", "192.168.1.50", "10.1.2.3"} {
		if err := verify(c, cert.Leaf, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := verify(c, cert.Leaf, "example.org"); err == nil {
		t.Error("the certificate was accepted for a name it does not carry")
	}
	if life := cert.Leaf.NotAfter.Sub(testTime); life > 398*24*time.Hour {
		t.Errorf("the certificate lives %s, more than 398 days", life)
	}
}

func TestPublicAddressIsRefused(t *testing.T) {
	c, _ := open(t, t.TempDir())
	for _, ip := range []string{"8.8.8.8", "100.64.0.1", "127.0.0.1", "2001:db8::1"} {
		if _, err := c.Issue(testTime, []net.IP{net.ParseIP(ip)}); err == nil {
			t.Errorf("a certificate was issued for %s", ip)
		}
	}
}

// TestDevicesWouldRefuseAForgery signs, with the issuer key, certificates the
// chain must not cover, and checks that a verifier refuses them. It is the
// limit written in the root that protects devices, not the server's manners.
func TestDevicesWouldRefuseAForgery(t *testing.T) {
	c, _ := open(t, t.TempDir())
	forge := func(name string, ip net.IP) *x509.Certificate {
		tmpl := &x509.Certificate{
			SerialNumber: mustSerial(t), NotBefore: testTime.Add(-time.Hour), NotAfter: testTime.AddDate(0, 0, 30),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		if name != "" {
			tmpl.DNSNames = []string{name}
		}
		if ip != nil {
			tmpl.IPAddresses = []net.IP{ip}
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, c.issuer, &c.issuerKey.PublicKey, c.issuerKey)
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := x509.ParseCertificate(der)
		return leaf
	}
	for _, name := range []string{"www.example.org", "notvila.internal", "vila.internal.example.org"} {
		var invalid x509.CertificateInvalidError
		if err := verify(c, forge(name, nil), name); !errors.As(err, &invalid) || invalid.Reason != x509.CANotAuthorizedForThisName {
			t.Errorf("%s: want a name constraint error, got %v", name, err)
		}
	}
	if err := verify(c, forge("", net.ParseIP("8.8.8.8")), "8.8.8.8"); err == nil {
		t.Error("a forged certificate for a public address was accepted")
	}
}

func TestOnlyTheIssuerKeyIsKept(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certificate")
	c, _ := open(t, dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("%d files were saved, want the root, the issuer and the issuer key", len(entries))
	}
	info, err := os.Stat(filepath.Join(dir, issuerKeyFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("issuer key: %v, mode %v", err, info.Mode().Perm())
	}

	again, created := open(t, dir)
	if created {
		t.Fatal("an existing chain was created again, which would lock every device out")
	}
	if again.Fingerprint() != c.Fingerprint() || again.Domain() != "vila.internal" {
		t.Fatal("the loaded chain is not the saved one")
	}
	if !again.issuerKey.PublicKey.Equal(c.issuer.PublicKey) || again.issuerKey.PublicKey.Equal(c.root.PublicKey) {
		t.Fatal("the saved key is not the issuer key")
	}
	if len(c.Fingerprint()) != 64 || len(c.Root()) == 0 {
		t.Fatal("the fingerprint or the root is missing")
	}
}

func mustSerial(t *testing.T) *big.Int {
	t.Helper()
	s, err := newSerial()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestAnIncompleteFolderIsNotReplaced loses one file of a chain that devices
// already trust. A new authority would silently lock every device out, so
// the server must refuse to start and leave the files as they are.
func TestAnIncompleteFolderIsNotReplaced(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certificate")
	open(t, dir)
	for _, lost := range []string{rootFile, issuerFile, issuerKeyFile} {
		kept, err := os.ReadFile(filepath.Join(dir, lost))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(dir, lost)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Open(dir, "vila.internal", "Vila", time.Now()); err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("with %s lost, Open answered %v", lost, err)
		}
		if got := filesIn(dir); len(got) != 2 {
			t.Fatalf("with %s lost, Open left %v in the folder", lost, got)
		}
		if err := os.WriteFile(filepath.Join(dir, lost), kept, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestABirthThatWasCutShortLeavesNoTrace stops the birth after the chain was
// built in its new folder but before the folder took its place.
func TestABirthThatWasCutShortLeavesNoTrace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certificate")
	if err := os.MkdirAll(dir+".new", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir+".new", rootFile), []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, created := open(t, dir)
	if !created {
		t.Fatal("the chain was not created after a birth that was cut short")
	}
	if got := filesIn(dir); len(got) != 3 {
		t.Fatalf("files in the folder: %v", got)
	}
	if _, err := os.Stat(dir + ".new"); err == nil {
		t.Fatal("the folder of the cut birth is still there")
	}
	if c.Domain() != "vila.internal" {
		t.Fatalf("domain %q", c.Domain())
	}
}

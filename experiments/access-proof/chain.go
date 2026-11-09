package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	caYears    = 30  // validity of the root and the issuer
	serverDays = 397 // validity of a server certificate, inside Apple's limits

	rootFile      = "root.pem"
	issuerFile    = "issuer.pem"
	issuerKeyFile = "issuer-key.pem"
)

// privateRanges are the only networks the community certificates may cover.
var privateRanges = []string{
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fe80::/10", "fc00::/7",
}

// chain is what a community server keeps after setup: the root certificate
// that devices install, and the issuer that signs server certificates.
// The root key is not here. It exists only inside newChain.
type chain struct {
	domain    string
	root      *x509.Certificate
	issuer    *x509.Certificate
	issuerKey *ecdsa.PrivateKey
}

// newChain creates the root and the issuer, both limited to the community
// domain and to private networks. The root key signs the issuer and is then
// dropped: it is never written to disk and never leaves this function.
func newChain(domain string, now time.Time) (*chain, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	rootTmpl, err := caTemplate("Earthler access proof root", domain, now)
	if err != nil {
		return nil, err
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, err
	}

	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	issuerTmpl, err := caTemplate("Earthler access proof issuer", domain, now)
	if err != nil {
		return nil, err
	}
	issuerTmpl.MaxPathLenZero = true // the issuer cannot create another issuer
	issuerDER, err := x509.CreateCertificate(rand.Reader, issuerTmpl, root, &issuerKey.PublicKey, rootKey)
	if err != nil {
		return nil, fmt.Errorf("create issuer: %w", err)
	}
	issuer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		return nil, err
	}
	return &chain{domain: domain, root: root, issuer: issuer, issuerKey: issuerKey}, nil
}

// caTemplate describes a certificate authority that may only sign for the
// community domain and for private network addresses.
func caTemplate(name, domain string, now time.Time) (*x509.Certificate, error) {
	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	var nets []*net.IPNet
	for _, r := range privateRanges {
		_, n, err := net.ParseCIDR(r)
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	return &x509.Certificate{
		SerialNumber:                serial,
		Subject:                     pkix.Name{CommonName: name + " (" + domain + ")"},
		NotBefore:                   now.Add(-time.Hour),
		NotAfter:                    now.AddDate(caYears, 0, 0),
		IsCA:                        true,
		BasicConstraintsValid:       true,
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         []string{domain},
		PermittedIPRanges:           nets,
	}, nil
}

// issue signs a server certificate for the given names and addresses. The
// issuer signs whatever it is asked to sign: the limits live in the root and
// issuer certificates, and it is the device that has to enforce them. The
// forged certificate test relies on exactly that.
func (c *chain) issue(now time.Time, names []string, ips []net.IP) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Earthler access proof"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(0, 0, serverDays),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.issuer, &key.PublicKey, c.issuerKey)
	if err != nil {
		return nil, fmt.Errorf("issue certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der, c.issuer.Raw}, PrivateKey: key, Leaf: leaf}, nil
}

// fingerprint is the SHA-256 of the root certificate, in uppercase hex.
// People compare its first characters before installing the certificate.
func (c *chain) fingerprint() string {
	sum := sha256.Sum256(c.root.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// save writes the two certificates and the issuer key. There is no root key
// to write.
func (c *chain) save(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(c.issuerKey)
	if err != nil {
		return err
	}
	files := []struct {
		name  string
		block *pem.Block
		mode  os.FileMode
	}{
		{rootFile, &pem.Block{Type: "CERTIFICATE", Bytes: c.root.Raw}, 0o644},
		{issuerFile, &pem.Block{Type: "CERTIFICATE", Bytes: c.issuer.Raw}, 0o644},
		{issuerKeyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}, 0o600},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), pem.EncodeToMemory(f.block), f.mode); err != nil {
			return err
		}
	}
	return nil
}

// loadChain reads a chain written by save. A missing file is reported as
// os.ErrNotExist, so the caller can create a new chain.
func loadChain(dir string) (*chain, error) {
	root, err := readCertificate(filepath.Join(dir, rootFile))
	if err != nil {
		return nil, err
	}
	issuer, err := readCertificate(filepath.Join(dir, issuerFile))
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, issuerKeyFile))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("issuer key: not a PEM file")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("issuer key: %w", err)
	}
	if len(root.PermittedDNSDomains) != 1 {
		return nil, errors.New("root certificate: expected exactly one permitted domain")
	}
	return &chain{domain: root.PermittedDNSDomains[0], root: root, issuer: issuer, issuerKey: key}, nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: not a PEM file", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

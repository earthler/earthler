// Package certificate gives a community its own certificate authority, so
// browsers accept the system over HTTPS on a network with no internet.
//
// A public certificate would not work: it has to be renewed over the
// internet every few months. So each community creates a root certificate,
// and each device installs it once. That is a dangerous power to hand out,
// which is why the root is limited to the community's own name and to
// private network addresses, and why its key is thrown away at birth.
package certificate

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

// Chain is what the server keeps: the root certificate that devices install
// and the issuer that signs the server's certificates. The root key is not
// here, or anywhere.
type Chain struct {
	domain    string
	root      *x509.Certificate
	issuer    *x509.Certificate
	issuerKey *ecdsa.PrivateKey
}

// Open loads the chain kept in dir, or creates it for domain on first use.
// created reports whether devices have a new root to install.
func Open(dir, domain string, now time.Time) (c *Chain, created bool, err error) {
	c, err = load(dir)
	if errors.Is(err, os.ErrNotExist) {
		if c, err = create(domain, now); err != nil {
			return nil, false, err
		}
		return c, true, c.save(dir)
	}
	return c, false, err
}

// create makes the root and the issuer. The root key signs the issuer and is
// then dropped: it is never written to disk and never leaves this function.
func create(domain string, now time.Time) (*Chain, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	rootTmpl, err := authority("Earthler community root", domain, now)
	if err != nil {
		return nil, err
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, fmt.Errorf("certificate: root: %w", err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, err
	}
	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	issuerTmpl, err := authority("Earthler community issuer", domain, now)
	if err != nil {
		return nil, err
	}
	issuerTmpl.MaxPathLenZero = true // the issuer cannot create another issuer
	issuerDER, err := x509.CreateCertificate(rand.Reader, issuerTmpl, root, &issuerKey.PublicKey, rootKey)
	if err != nil {
		return nil, fmt.Errorf("certificate: issuer: %w", err)
	}
	issuer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		return nil, err
	}
	return &Chain{domain: domain, root: root, issuer: issuer, issuerKey: issuerKey}, nil
}

// authority describes a certificate authority that may only sign for the
// community domain and for private network addresses.
func authority(name, domain string, now time.Time) (*x509.Certificate, error) {
	serial, err := newSerial()
	if err != nil {
		return nil, err
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
		PermittedIPRanges:           privateNetworks(),
	}, nil
}

func privateNetworks() []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(privateRanges))
	for _, r := range privateRanges {
		_, n, err := net.ParseCIDR(r)
		if err != nil {
			panic(err) // the list above is fixed
		}
		nets = append(nets, n)
	}
	return nets
}

// Issue signs a certificate for the community's own name and for the given
// addresses of this machine. It refuses any address that is not on a private
// network: the limits written in the root would make devices reject it, and
// the server should never even try.
func (c *Chain) Issue(now time.Time, ips []net.IP) (*tls.Certificate, error) {
	for _, ip := range ips {
		allowed := false
		for _, n := range privateNetworks() {
			allowed = allowed || n.Contains(ip)
		}
		if !allowed {
			return nil, fmt.Errorf("certificate: %s is not a private network address", ip)
		}
	}
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
		Subject:               pkix.Name{CommonName: c.domain},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(0, 0, serverDays),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{c.domain},
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.issuer, &key.PublicKey, c.issuerKey)
	if err != nil {
		return nil, fmt.Errorf("certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der, c.issuer.Raw}, PrivateKey: key, Leaf: leaf}, nil
}

// Domain is the name the chain was created for, such as vila.internal.
func (c *Chain) Domain() string { return c.domain }

// Root is the certificate that devices install, in the DER form they expect.
func (c *Chain) Root() []byte { return c.root.Raw }

// Fingerprint is the SHA-256 of the root, in uppercase hex. People compare
// its first characters with a printed poster before they install the root.
func (c *Chain) Fingerprint() string {
	sum := sha256.Sum256(c.root.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func (c *Chain) save(dir string) error {
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

func load(dir string) (*Chain, error) {
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
		return nil, errors.New("certificate: the issuer key is not a PEM file")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("certificate: issuer key: %w", err)
	}
	if len(root.PermittedDNSDomains) != 1 {
		return nil, errors.New("certificate: the root is not limited to one domain")
	}
	return &Chain{domain: root.PermittedDNSDomains[0], root: root, issuer: issuer, issuerKey: key}, nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("certificate: %s is not a PEM file", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

func newSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

// LocalAddresses returns the private IPv4 addresses of this machine.
func LocalAddresses() ([]net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var ips []net.IP
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok {
			if ip := ipnet.IP.To4(); ip != nil && ip.IsPrivate() {
				ips = append(ips, ip)
			}
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("certificate: this machine has no private network address")
	}
	return ips, nil
}

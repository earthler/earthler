package certificate

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The chain is kept as three PEM files in the secrets folder. The root and
// the issuer are public; the issuer key is the one secret, readable only by
// the server's own user.

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

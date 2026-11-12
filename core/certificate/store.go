package certificate

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/earthler/earthler/core/durable"
)

// The chain is kept as three PEM files in the secrets folder. The root and
// the issuer are public; the issuer key is the one secret, readable only by
// the server's own user.

// save writes the three files into a new folder and then puts the folder in
// its place, so a power cut during the birth of the community leaves either
// the whole chain or nothing, never part of it.
func (c *Chain) save(dir string) error {
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
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	building := dir + ".new"
	if err := os.RemoveAll(building); err != nil {
		return err
	}
	if err := os.Mkdir(building, 0o700); err != nil {
		return err
	}
	for _, f := range files {
		if err := durable.WriteFile(filepath.Join(building, f.name), pem.EncodeToMemory(f.block), f.mode); err != nil {
			return err
		}
	}
	// An empty folder may already be there; a folder with files in it is
	// never replaced.
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(building, dir); err != nil {
		return err
	}
	return durable.SyncDir(parent)
}

// filesIn lists which of the chain's files are in dir.
func filesIn(dir string) []string {
	var present []string
	for _, name := range []string{rootFile, issuerFile, issuerKeyFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			present = append(present, name)
		}
	}
	return present
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

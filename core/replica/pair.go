package replica

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// PairOptions is what a person gives to pair a second machine.
type PairOptions struct {
	URL         string // the secure address of the main machine, such as https://192.168.1.50
	PlainURL    string // the address of its welcome page, where the root is fetched
	Code        string // the pairing code the main machine printed
	Fingerprint string // the fingerprint of the community root it printed
	Label       string // a name for this machine, shown on the main machine
	Dir         string // where this machine keeps the copy
}

// Fingerprint is the SHA-256 of a root, in the form people compare: uppercase hex.
func Fingerprint(root []byte) string {
	sum := sha256.Sum256(root)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func tidyFingerprint(text string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", ":", "", "-", "").Replace(text))
}

// Pair brings a new second machine in. It fetches the community root from the
// welcome page, trusts it only if its fingerprint is the one the person read
// from the main machine, and trades the pairing code for a token over a
// connection that only that root can vouch for.
func Pair(ctx context.Context, o PairOptions) (*Follower, error) {
	root, err := fetchRoot(ctx, o.PlainURL)
	if err != nil {
		return nil, err
	}
	if Fingerprint(root) != tidyFingerprint(o.Fingerprint) {
		return nil, errors.New("replica: the certificate at that address is not the one with the fingerprint you gave")
	}
	client, err := pinned(root)
	if err != nil {
		return nil, err
	}
	form := url.Values{"code": {o.Code}, "label": {o.Label}}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(o.URL, "/")+"/replica/pair", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("replica: the main machine could not be reached: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("replica: pairing was refused (%d): %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	f := &Follower{dir: o.Dir, client: client, state: state{
		URL: strings.TrimSuffix(o.URL, "/"), Token: strings.TrimSpace(string(body)), Root: root, Held: map[string]File{}}}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, err
	}
	return f, f.save()
}

func fetchRoot(ctx context.Context, plainURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(plainURL, "/")+"/certificate.crt", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("replica: the welcome page could not be reached: %w", err)
	}
	defer res.Body.Close()
	root, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil || res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("replica: the certificate could not be read (%d)", res.StatusCode)
	}
	return root, nil
}

// pinned makes a client that trusts the community root and nothing else, and
// asks for the community's own name, which the root is limited to.
func pinned(root []byte) (*http.Client, error) {
	cert, err := x509.ParseCertificate(root)
	if err != nil || len(cert.PermittedDNSDomains) == 0 {
		return nil, errors.New("replica: that is not a community root")
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: cert.PermittedDNSDomains[0], MinVersion: tls.VersionTLS12},
	}}, nil
}

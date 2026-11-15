package machines

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/earthler/earthler/core/durable"
)

// Peers are the addresses of the other machines, by number, as this machine
// last learned them. They live beside the data and not in the journal,
// because an address is a fact about the network today, not about the
// community.
type Peers struct {
	path string
	mu   sync.Mutex
	urls map[uint64]string
}

// OpenPeers reads the addresses kept in path. A missing file is no address.
func OpenPeers(path string) (*Peers, error) {
	p := &Peers{path: path, urls: map[uint64]string{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	var saved map[string]string
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, fmt.Errorf("machines: %s is damaged: %w", path, err)
	}
	for k, v := range saved {
		n, err := strconv.ParseUint(k, 10, 64)
		if err != nil || CheckAddress(v) != nil {
			return nil, fmt.Errorf("machines: %s has %q for %q", path, v, k)
		}
		p.urls[n] = v
	}
	return p, nil
}

// CheckAddress says whether text is an address another machine can be
// reached at: https, a host, and nothing after it.
func CheckAddress(text string) error {
	u, err := url.Parse(text)
	if err != nil || u.Scheme != "https" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return fmt.Errorf("machines: %q is not an https address of a machine", text)
	}
	return nil
}

// Set keeps the address of machine n. It says whether anything changed.
func (p *Peers) Set(n uint64, address string) (bool, error) {
	if err := CheckAddress(address); err != nil {
		return false, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.urls[n] == address {
		return false, nil
	}
	p.urls[n] = address
	saved := map[string]string{}
	for k, v := range p.urls {
		saved[strconv.FormatUint(k, 10)] = v
	}
	raw, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return false, err
	}
	return true, durable.WriteFile(p.path, raw, 0o600)
}

// Get returns the address of machine n.
func (p *Peers) Get(n uint64) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.urls[n]
	return a, ok
}

// All returns every address that is known, by machine.
func (p *Peers) All() map[uint64]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[uint64]string, len(p.urls))
	for k, v := range p.urls {
		out[k] = v
	}
	return out
}

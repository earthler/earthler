// Package replica keeps a second machine up to date. The second machine is
// the same program in a mode that opens no screens: it pulls, over HTTPS and
// with a token of its own, the encrypted copy that the main machine keeps of
// the journal and the secrets, and stores it as it came. It never holds the
// copy key, so a machine in another house reads nothing. To take over, a
// person types the copy key and restores the copy, which is the restore that
// the copies already have.
//
// The design is in docs/design/replica.md.
package replica

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/earthler/earthler/core/durable"
)

var (
	// ErrBadCode is a pairing code that is wrong, was already used, or has expired.
	ErrBadCode = errors.New("replica: the pairing code is wrong, was used or has expired")
	// ErrTooMany stops guessing: too many wrong answers in a short time.
	ErrTooMany = errors.New("replica: too many wrong answers, wait a minute")
)

const (
	pairingFile = "pairing.json"
	tokensFile  = "tokens.json"

	// CodeLife is how long a pairing code works.
	CodeLife = 10 * time.Minute

	maxFailures = 5
	failureSpan = time.Minute
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// pending is the pairing code that was asked for and not used yet. It lives
// in a file so that the program that prints the code and the server that
// checks it, which are two processes, share it.
type pending struct {
	Hash    string    `json:"hash"`
	Expires time.Time `json:"expires"`
}

type token struct {
	ID    string    `json:"id"`
	Hash  string    `json:"hash"`
	Label string    `json:"label"`
	Made  time.Time `json:"made"`
}

// Registry holds the pairing code and the tokens of the second machines of
// one community. Only the hashes of codes and tokens are written.
type Registry struct {
	dir string

	mu       sync.Mutex
	failures map[string][]time.Time
}

// OpenRegistry returns the registry kept in dir, creating the folder.
func OpenRegistry(dir string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Registry{dir: dir, failures: map[string][]time.Time{}}, nil
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// tidy reads a code or a token as a person typed it: case, spaces and
// hyphens do not matter, and the letters that look like digits are the digits.
func tidy(text string) string {
	text = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\n", "", "\t", "").Replace(text))
	return strings.NewReplacer("0", "O", "1", "I").Replace(text)
}

func randomText(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return encoding.EncodeToString(raw), nil
}

// NewCode makes the code that pairs one second machine and writes it for the
// running server to find. The code works once, for ten minutes.
func NewCode(dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	text, err := randomText(8) // 13 letters: 64 bits
	if err != nil {
		return "", err
	}
	code := text[:12]
	raw, err := json.Marshal(pending{Hash: digest(tidy(code)), Expires: now.Add(CodeLife)})
	if err != nil {
		return "", err
	}
	if err := durable.WriteFile(filepath.Join(dir, pairingFile), raw, 0o600); err != nil {
		return "", err
	}
	return code[0:4] + "-" + code[4:8] + "-" + code[8:12], nil
}

// tooMany says whether who has failed too often lately, and counts one more
// failure when add is set.
func (r *Registry) tooMany(who string, now time.Time, add bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	recent := r.failures[who][:0]
	for _, t := range r.failures[who] {
		if now.Sub(t) < failureSpan {
			recent = append(recent, t)
		}
	}
	if add {
		recent = append(recent, now)
	}
	if len(recent) == 0 {
		delete(r.failures, who)
	} else {
		r.failures[who] = recent
	}
	if len(r.failures) > 4096 { // never grow without a limit
		r.failures = map[string][]time.Time{who: recent}
	}
	return len(recent) >= maxFailures
}

// Redeem trades a pairing code for a token. The code is spent: a second try
// with it fails. who says where the try came from, for the limit on guesses.
func (r *Registry) Redeem(code, label, who string, now time.Time) (string, error) {
	if r.tooMany(who, now, false) {
		return "", ErrTooMany
	}
	raw, err := os.ReadFile(filepath.Join(r.dir, pairingFile))
	var p pending
	if err != nil || json.Unmarshal(raw, &p) != nil || now.After(p.Expires) ||
		subtle.ConstantTimeCompare([]byte(p.Hash), []byte(digest(tidy(code)))) != 1 {
		r.tooMany(who, now, true)
		return "", ErrBadCode
	}
	if err := os.Remove(filepath.Join(r.dir, pairingFile)); err != nil {
		return "", err
	}
	secret, err := randomText(32)
	if err != nil {
		return "", err
	}
	id, err := randomText(5)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens, err := r.read()
	if err != nil {
		return "", err
	}
	label = strings.Join(strings.Fields(label), " ")
	if len([]rune(label)) > 60 {
		label = string([]rune(label)[:60])
	}
	tokens = append(tokens, token{ID: id, Hash: digest(secret), Label: label, Made: now})
	return secret, r.write(tokens)
}

// Authorize says whether a token belongs to a second machine, and which one.
func (r *Registry) Authorize(secret, who string, now time.Time) (string, bool) {
	if r.tooMany(who, now, false) {
		return "", false
	}
	r.mu.Lock()
	tokens, err := r.read()
	r.mu.Unlock()
	sum := digest(strings.TrimSpace(secret))
	found, id := 0, ""
	if err == nil {
		for _, t := range tokens {
			// Every token is compared, so the time does not say which one matched.
			if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(sum)) == 1 {
				found, id = 1, t.ID
			}
		}
	}
	if found == 0 {
		r.tooMany(who, now, true)
		return "", false
	}
	return id, true
}

// Revoke takes a token away. The machine that had it can pull no more.
func (r *Registry) Revoke(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens, err := r.read()
	if err != nil {
		return err
	}
	kept := tokens[:0]
	for _, t := range tokens {
		if t.ID != id {
			kept = append(kept, t)
		}
	}
	return r.write(kept)
}

// Machine is a second machine that was paired.
type Machine struct {
	ID    string
	Label string
	Made  time.Time
}

// Machines lists the tokens that exist, without the secrets.
func (r *Registry) Machines() ([]Machine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens, err := r.read()
	out := make([]Machine, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, Machine{ID: t.ID, Label: t.Label, Made: t.Made})
	}
	return out, err
}

func (r *Registry) read() ([]token, error) {
	raw, err := os.ReadFile(filepath.Join(r.dir, tokensFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tokens []token
	return tokens, json.Unmarshal(raw, &tokens)
}

func (r *Registry) write(tokens []token) error {
	raw, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	return durable.WriteFile(filepath.Join(r.dir, tokensFile), raw, 0o600)
}

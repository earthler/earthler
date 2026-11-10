// Package copies makes encrypted copies of what cannot be rebuilt: the
// journal and the secrets. Everything leaves the machine encrypted with the
// copy key, so a copy can sit on a neighbour's disk without the neighbour
// reading it, and a copy that was changed or cut on the way is refused.
//
// The format is described in docs/design/copies.md.
package copies

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Key encrypts every copy of one community. It is made once, when the
// community is born, and the person writes it down: a copy can only be read
// back with it, on this machine or on another.
type Key [32]byte

const keyFile = "copy.key"

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// LoadKey reads the copy key kept in the secrets folder, creating it the
// first time.
func LoadKey(dir string) (Key, error) {
	path := filepath.Join(dir, keyFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		var k Key
		if _, err := rand.Read(k[:]); err != nil {
			return k, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return k, err
		}
		return k, os.WriteFile(path, []byte(k.Text()+"\n"), 0o600)
	}
	if err != nil {
		return Key{}, err
	}
	return ParseKey(string(raw))
}

// Text writes the key the way a person copies it to paper: groups of four
// letters and digits, with no letter that looks like a digit.
func (k Key) Text() string {
	s := encoding.EncodeToString(k[:])
	groups := make([]string, 0, len(s)/4+1)
	for i := 0; i < len(s); i += 4 {
		groups = append(groups, s[i:min(i+4, len(s))])
	}
	return strings.Join(groups, "-")
}

// ParseKey reads a key as a person typed it: case, spaces and hyphens do
// not matter, and a 0 or a 1 is taken for the letter it looks like.
func ParseKey(text string) (Key, error) {
	var k Key
	clean := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r), r == '-':
			return -1
		case r == '0':
			return 'O'
		case r == '1':
			return 'I'
		}
		return unicode.ToUpper(r)
	}, text)
	raw, err := encoding.DecodeString(clean)
	if err != nil || len(raw) != len(k) {
		return k, errors.New("copies: that is not a copy key")
	}
	copy(k[:], raw)
	return k, nil
}

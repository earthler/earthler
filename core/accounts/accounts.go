// Package accounts knows who the residents are and which browser belongs to
// whom.
//
// This is the first, minimal form: a resident signs up with a name and stays
// signed in on that browser. There is no passphrase yet, so an account
// cannot be opened from a second device and nothing personal is protected.
// Both arrive with the personal layer.
package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/earthler/earthler/core/durable"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/permissions"
	"github.com/earthler/earthler/core/roster"
	"github.com/earthler/earthler/core/scribe"
)

// Area is the index of accounts and devices.
var Area = index.Area{
	Name: "accounts",
	Schema: `
		CREATE TABLE accounts (number INTEGER PRIMARY KEY, name TEXT NOT NULL, folded TEXT NOT NULL UNIQUE,
			joined INTEGER NOT NULL DEFAULT 0, request TEXT NOT NULL DEFAULT '');
		CREATE INDEX accounts_by_request ON accounts (request);
		` + roster.Schema + `
		CREATE TABLE devices (number INTEGER PRIMARY KEY, account INTEGER NOT NULL, counter INTEGER NOT NULL DEFAULT 0);`,
	Apply: apply,
}

func apply(tx *sql.Tx, entry index.Entry) error {
	if _, err := roster.Apply(tx, entry); err != nil {
		return err
	}
	e, err := event.Decode(entry.Content)
	if err != nil {
		return nil
	}
	switch e.Type {
	case event.AccountCreated:
		var d event.AccountCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		// Sign up checks the name first. If two entries with the same name
		// exist anyway, the first one keeps it.
		if _, err = tx.Exec(`INSERT INTO accounts (number, name, folded, joined, request) VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			entry.ID, d.Name, fold(d.Name), e.Time, d.Request); err == nil && d.FirstDevice {
			_, err = tx.Exec(`INSERT INTO devices (number, account) VALUES (?, ?) ON CONFLICT DO NOTHING`, entry.ID, entry.ID)
		}
	case event.DeviceCreated:
		var d event.DeviceCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO devices (number, account) VALUES (?, ?) ON CONFLICT DO NOTHING`, entry.ID, d.Account)
	}
	return err
}

// MaxName is the longest name, in characters.
const MaxName = 40

var (
	ErrNameEmpty   = errors.New("accounts: the name is empty")
	ErrNameLong    = errors.New("accounts: the name is too long")
	ErrNameTaken   = errors.New("accounts: the name is taken")
	ErrNameInvalid = errors.New("accounts: the name has characters that cannot be used")
)

// ErrNoSession is what a request with no valid cookie gets. Any other error
// from looking a session up is the system failing, which is not the same as
// nobody being signed in.
var ErrNoSession = errors.New("accounts: nobody is signed in")

// checkName says whether a name that was already tidied (spaces collapsed)
// can be shown to everyone. Only letters, digits, punctuation, symbols and
// the plain space are allowed. Everything else is refused because it cannot
// be seen, or is shown in a way that fools: zero-width characters and
// direction overrides make two names look the same, and marks are written
// in more than one way (an accent can be a part of the letter or a mark
// after it), so the same name could be taken twice. A name in a script that
// needs joiners or marks has to wait until names are compared after
// normalisation.
func checkName(name string) error {
	if !utf8.ValidString(name) {
		return ErrNameInvalid
	}
	for _, r := range name {
		switch {
		case r == ' ':
		case r == utf8.RuneError:
			return ErrNameInvalid
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsPunct(r), unicode.IsSymbol(r):
		default:
			return ErrNameInvalid
		}
	}
	return nil
}

// Session says who is behind a request.
type Session struct {
	Account uint64
	Device  uint64
	Name    string
	Role    permissions.Role
}

// Service signs residents up and recognises their browsers.
type Service struct {
	index   *index.Index
	scribe  *scribe.Scribe
	key     []byte
	counter Counter    // the machine that counts, when it is another one
	signup  sync.Mutex // one sign up at a time, so a name is checked and taken together
	roles   sync.Mutex // one change of role at a time, so the last administrator is counted right
}

// New returns the service. key signs the session cookies.
func New(ix *index.Index, s *scribe.Scribe, key []byte) *Service {
	return &Service{index: ix, scribe: s, key: key}
}

// SignUp creates an account and a device for it, and returns the cookie that
// keeps this browser signed in. A name is counted: on a machine that does
// not count, the machine that counts takes it, and the answer waits until
// the account has reached this machine, so the next page knows it.
func (s *Service) SignUp(ctx context.Context, name string, secure bool) (*http.Cookie, error) {
	name, err := tidyName(name)
	if err != nil {
		return nil, err
	}
	if s.counter == nil {
		device, err := s.Register(ctx, name, "")
		if err != nil {
			return nil, err
		}
		return s.cookie(device, 0, secure), nil
	}
	request, err := mark()
	if err != nil {
		return nil, err
	}
	device, err := s.counter.SignUp(ctx, name, request)
	switch {
	case errors.Is(err, ErrUnsure):
		// The request went and the answer did not come back. If the account
		// was made, it arrives with the mark this machine chose.
		if device, err = s.found(ctx, `SELECT number FROM accounts WHERE request = ?`, request); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		// The account exists. The next page knows it once it has reached
		// this machine, which is a moment; waiting longer changes nothing.
		s.arrives(ctx, `SELECT count(*) FROM devices WHERE number = ?`, device)
	}
	return s.cookie(device, 0, secure), nil
}

// Register creates an account and its first device on the machine that
// counts, and returns the device. The account and its first device are one
// entry of the journal, so there is never one without the other.
func (s *Service) Register(ctx context.Context, name, request string) (uint64, error) {
	name, err := tidyName(name)
	if err != nil {
		return 0, err
	}
	s.signup.Lock()
	defer s.signup.Unlock()
	var taken int
	if err := s.index.DB().QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE folded = ?`, fold(name)).Scan(&taken); err != nil {
		return 0, err
	}
	if taken > 0 {
		return 0, ErrNameTaken
	}
	if len(request) > 64 {
		request = request[:64]
	}
	content, err := event.New(event.AccountCreated, 0, 0, time.Now(), event.AccountCreatedData{Name: name, FirstDevice: true, Request: request})
	if err != nil {
		return 0, err
	}
	entries, err := s.scribe.Write(ctx, content)
	if err != nil {
		return 0, err
	}
	return entries[0].ID, nil
}

// tidyName collapses the spaces of a name and checks it.
func tidyName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrNameInvalid
	}
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return "", ErrNameEmpty
	case len([]rune(name)) > MaxName:
		return "", ErrNameLong
	}
	return name, checkName(name)
}

// fold makes names comparable: "Ana Lima" and "ana  lima" are the same name.
func fold(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// LoadKey reads the key that signs the cookies, creating it on first use. A
// key file that is there but is not a key is an error, never a reason to
// make a new key: that would sign everyone out without anyone knowing why.
func LoadKey(dir string) ([]byte, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "session.key")
	key, err := os.ReadFile(path)
	switch {
	case err == nil && len(key) == 32:
		return key, nil
	case err == nil:
		return nil, fmt.Errorf("accounts: %s has %d bytes and is not a session key: restore it from a copy, or remove it to sign everyone out", path, len(key))
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, durable.WriteFile(path, key, 0o600)
}

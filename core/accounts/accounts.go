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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/earthler/earthler/core/durable"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/scribe"
)

// Area is the index of accounts and devices.
var Area = index.Area{
	Name: "accounts",
	Schema: `
		CREATE TABLE accounts (number INTEGER PRIMARY KEY, name TEXT NOT NULL, folded TEXT NOT NULL UNIQUE);
		CREATE TABLE devices (number INTEGER PRIMARY KEY, account INTEGER NOT NULL, counter INTEGER NOT NULL DEFAULT 0);`,
	Apply: apply,
}

func apply(tx *sql.Tx, entry journal.Entry) error {
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
		if _, err = tx.Exec(`INSERT INTO accounts (number, name, folded) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
			entry.Number, d.Name, fold(d.Name)); err == nil && d.FirstDevice {
			_, err = tx.Exec(`INSERT INTO devices (number, account) VALUES (?, ?) ON CONFLICT DO NOTHING`, entry.Number, entry.Number)
		}
	case event.DeviceCreated:
		var d event.DeviceCreatedData
		if e.Read(&d) != nil {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO devices (number, account) VALUES (?, ?) ON CONFLICT DO NOTHING`, entry.Number, d.Account)
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
}

// Service signs residents up and recognises their browsers.
type Service struct {
	index  *index.Index
	scribe *scribe.Scribe
	key    []byte
	signup sync.Mutex // one sign up at a time, so a name is checked and taken together
}

// New returns the service. key signs the session cookies.
func New(ix *index.Index, s *scribe.Scribe, key []byte) *Service {
	return &Service{index: ix, scribe: s, key: key}
}

// SignUp creates an account and a device for it, and returns the cookie that
// keeps this browser signed in. The account and its first device are one
// entry of the journal, so there is never one without the other.
func (s *Service) SignUp(ctx context.Context, name string, secure bool) (*http.Cookie, error) {
	if !utf8.ValidString(name) {
		return nil, ErrNameInvalid
	}
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return nil, ErrNameEmpty
	case len([]rune(name)) > MaxName:
		return nil, ErrNameLong
	}
	if err := checkName(name); err != nil {
		return nil, err
	}

	s.signup.Lock()
	defer s.signup.Unlock()
	var taken int
	if err := s.index.DB().QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE folded = ?`, fold(name)).Scan(&taken); err != nil {
		return nil, err
	}
	if taken > 0 {
		return nil, ErrNameTaken
	}

	content, err := event.New(event.AccountCreated, 0, 0, time.Now(), event.AccountCreatedData{Name: name, FirstDevice: true})
	if err != nil {
		return nil, err
	}
	entries, err := s.scribe.Write(ctx, content)
	if err != nil {
		return nil, err
	}
	return s.cookie(entries[0].Number, 0, secure), nil
}

const cookieName = "earthler"

// cookie builds the session cookie: the device, its counter, and a signature
// of the two made with the server's key. There is no table of sessions, so
// rebuilding the indexes signs nobody out.
func (s *Service) cookie(device, counter uint64, secure bool) *http.Cookie {
	value := fmt.Sprintf("%d.%d", device, counter)
	return &http.Cookie{
		Name:     cookieName,
		Value:    value + "." + s.sign(value),
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (s *Service) sign(value string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// Session returns who sent the request, if the cookie is one this server
// signed and the device still exists with the same counter.
func (s *Service) Session(r *http.Request) (Session, bool) {
	sess, err := s.Lookup(r)
	return sess, err == nil
}

// Lookup is Session with the reason: ErrNoSession when the request is from
// nobody, and the database's own error when the lookup itself failed.
func (s *Service) Lookup(r *http.Request) (Session, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return Session{}, ErrNoSession
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 || !hmac.Equal([]byte(s.sign(parts[0]+"."+parts[1])), []byte(parts[2])) {
		return Session{}, ErrNoSession
	}
	device, err1 := strconv.ParseUint(parts[0], 10, 63)
	counter, err2 := strconv.ParseUint(parts[1], 10, 63)
	if err1 != nil || err2 != nil {
		return Session{}, ErrNoSession
	}
	sess := Session{Device: device}
	err = s.index.DB().QueryRowContext(r.Context(), `
		SELECT a.number, a.name FROM devices d JOIN accounts a ON a.number = d.account
		WHERE d.number = ? AND d.counter = ?`, device, counter).Scan(&sess.Account, &sess.Name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Session{}, ErrNoSession
	case err != nil:
		return Session{}, err
	}
	return sess, nil
}

type contextKey struct{}

// Require lets a request through only when it comes from a signed in
// browser. Everyone else is sent to the sign up screen.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.Lookup(r)
		switch {
		case errors.Is(err, ErrNoSession):
			http.Redirect(w, r, "/sign-up", http.StatusSeeOther)
			return
		case err != nil:
			// The system failed. Sending a resident to the sign up screen
			// would look like being signed out.
			log.Printf("accounts: %v", err)
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, sess)))
	})
}

// From returns the session that Require stored in the request.
func From(ctx context.Context) Session {
	sess, _ := ctx.Value(contextKey{}).(Session)
	return sess
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

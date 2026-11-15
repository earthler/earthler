package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

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
		SELECT a.number, a.name, r.role FROM devices d
		JOIN accounts a ON a.number = d.account
		JOIN roster r ON r.account = a.number
		WHERE d.number = ? AND d.counter = ?`, device, counter).Scan(&sess.Account, &sess.Name, &sess.Role)
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

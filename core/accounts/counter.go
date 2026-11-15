package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/earthler/earthler/core/permissions"
)

// Counter is the machine that counts, as a machine that does not count sees
// it. What can have only one owner or one result, a name or a role, is
// decided there and nowhere else.
//
// A sign up carries a random mark that the account keeps, so the machine
// that asked can recognise it if the answer is lost.
type Counter interface {
	SignUp(ctx context.Context, name, request string) (device uint64, err error)
	ChangeRole(ctx context.Context, actor Session, target uint64, role permissions.Role) error
}

var (
	// ErrNotNow is what a counted act gets when the machine that counts
	// cannot be reached, or answered that it could not do it. The act can
	// be asked again later.
	ErrNotNow = errors.New("accounts: this has to be decided by the machine that counts, which cannot be reached now")

	// ErrUnsure says the request reached the machine that counts and the
	// answer did not come back, so the act may have been done.
	ErrUnsure = errors.New("accounts: the machine that counts was asked and did not answer")
)

// mark is a random text for a request.
func mark() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// found waits for query, which returns one id, to find a row, and returns
// ErrNotNow when none comes in time.
func (s *Service) found(ctx context.Context, query string, args ...any) (uint64, error) {
	if err := s.arrives(ctx, `SELECT count(*) FROM (`+query+`)`, args...); err != nil {
		return 0, err
	}
	var id uint64
	return id, s.index.DB().QueryRowContext(ctx, query, args...).Scan(&id)
}

// CountAt makes this service send what is counted to another machine. The
// machine that counts never calls it.
func (s *Service) CountAt(c Counter) { s.counter = c }

// arriveWait is how long a machine waits for what the machine that counts
// wrote to reach its own indexes.
var arriveWait = 10 * time.Second

// arrives waits until query, which counts rows, finds one for arg in the
// index. What the machine that counts wrote reaches this machine a moment
// later, and the next page has to know it.
func (s *Service) arrives(ctx context.Context, query string, args ...any) error {
	deadline := time.Now().Add(arriveWait)
	for {
		var n int
		if err := s.index.DB().QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrNotNow
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Device returns the session of a device, for an act that another machine
// asks the machine that counts to decide: the role is the one this machine
// knows, never one the other machine claims.
func (s *Service) Device(ctx context.Context, device uint64) (Session, error) {
	sess := Session{Device: device}
	err := s.index.DB().QueryRowContext(ctx, `
		SELECT a.number, a.name, r.role FROM devices d
		JOIN accounts a ON a.number = d.account
		JOIN roster r ON r.account = a.number
		WHERE d.number = ?`, device).Scan(&sess.Account, &sess.Name, &sess.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	return sess, err
}

package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/earthler/earthler/core/permissions"
)

// Counter is the machine that counts, as a machine that does not count sees
// it. What can have only one owner or one result, a name or a role, is
// decided there and nowhere else.
type Counter interface {
	SignUp(ctx context.Context, name string) (device uint64, err error)
	ChangeRole(ctx context.Context, actor Session, target uint64, role permissions.Role) error
}

// ErrNotNow is what a counted act gets when the machine that counts cannot
// be reached. Nothing was done, and the act can be asked again later.
var ErrNotNow = errors.New("accounts: this has to be decided by the machine that counts, which cannot be reached now")

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

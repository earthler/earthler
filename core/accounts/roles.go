package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/permissions"
)

var (
	// ErrNotAllowed is what someone gets when they try what their role does
	// not let them do.
	ErrNotAllowed = errors.New("accounts: the role does not allow this")
	// ErrNoSuchAccount is a role asked for an account that does not exist.
	ErrNoSuchAccount = errors.New("accounts: there is no such account")
	// ErrBadRole is a role that does not exist, or one that is not given by hand.
	ErrBadRole = errors.New("accounts: that role cannot be given")
	// ErrLastAdministrator stops a community from being left with nobody who
	// can change roles.
	ErrLastAdministrator = errors.New("accounts: the community must keep at least one administrator")
)

// ChangeRole gives the account target the role role, on behalf of actor.
// Only an administrator may, only roles that are given by hand can be given,
// a child keeps the role of a child until guardians exist, and the last
// administrator cannot be demoted. Giving the role someone already has
// writes nothing.
func (s *Service) ChangeRole(ctx context.Context, actor Session, target uint64, role permissions.Role) error {
	if !permissions.Can(actor.Role, permissions.ChangeRoles) {
		return ErrNotAllowed
	}
	if !permissions.Assignable(role) {
		return ErrBadRole
	}

	s.roles.Lock()
	defer s.roles.Unlock()
	var current permissions.Role
	err := s.index.DB().QueryRowContext(ctx, `SELECT role FROM roster WHERE account = ?`, target).Scan(&current)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNoSuchAccount
	case err != nil:
		return err
	case current == permissions.Child:
		return ErrNotAllowed
	case current == role:
		return nil
	}
	if current == permissions.Administrator {
		var administrators int
		if err := s.index.DB().QueryRowContext(ctx, `SELECT count(*) FROM roster WHERE role = 'administrator'`).Scan(&administrators); err != nil {
			return err
		}
		if administrators <= 1 {
			return ErrLastAdministrator
		}
	}
	content, err := event.New(event.RoleChanged, actor.Account, actor.Device, time.Now(),
		event.RoleChangedData{Account: target, Role: string(role)})
	if err != nil {
		return err
	}
	_, err = s.scribe.Write(ctx, content)
	return err
}

// Resident is one line of the list of residents.
type Resident struct {
	Number uint64
	Name   string
	Role   permissions.Role
	Joined time.Time
}

// Residents lists everyone, in the order they joined.
func (s *Service) Residents(ctx context.Context) ([]Resident, error) {
	rows, err := s.index.DB().QueryContext(ctx, `SELECT a.number, a.name, r.role, a.joined FROM accounts a JOIN roster r ON r.account = a.number ORDER BY a.number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []Resident
	for rows.Next() {
		var r Resident
		var joined int64
		if err := rows.Scan(&r.Number, &r.Name, &r.Role, &joined); err != nil {
			return nil, err
		}
		r.Joined = time.UnixMilli(joined)
		all = append(all, r)
	}
	return all, rows.Err()
}

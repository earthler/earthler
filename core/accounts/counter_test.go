package accounts

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/permissions"
	"github.com/earthler/earthler/core/scribe"
)

// lostAnswer is a machine that counts whose answers are lost on the way: it
// does the act or not, and says nothing either way.
type lostAnswer struct {
	does *Service // the machine that counts, when it does the act
}

func (c lostAnswer) SignUp(ctx context.Context, name, request string) (uint64, error) {
	if c.does != nil {
		if _, err := c.does.Register(ctx, name, request); err != nil {
			return 0, err
		}
	}
	return 0, ErrUnsure
}

func (c lostAnswer) ChangeRole(ctx context.Context, actor Session, target uint64, role permissions.Role) error {
	return ErrUnsure
}

func service(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	j, err := journal.Open(filepath.Join(root, "journal"), journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(filepath.Join(root, "indexes"), Area)
	if err != nil {
		t.Fatal(err)
	}
	s, err := scribe.Start(j, ix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); ix.Close(); j.Close() })
	return New(ix, s, make([]byte, 32))
}

// A sign up whose answer was lost finds its account by the mark it chose,
// and says not now only when no account with that mark comes.
func TestASignUpWithALostAnswerFindsItsAccount(t *testing.T) {
	wait := arriveWait
	arriveWait = 300 * time.Millisecond
	t.Cleanup(func() { arriveWait = wait })

	// One service plays both machines: the account it makes is in the
	// index the sign up looks in, as it is once the entry has arrived.
	s := service(t)
	s.CountAt(lostAnswer{does: s})
	cookie, err := s.SignUp(context.Background(), "Ana", false)
	if err != nil {
		t.Fatalf("the account was made and the sign up said %v", err)
	}
	if cookie.Value == "" {
		t.Fatal("no cookie")
	}

	s.CountAt(lostAnswer{})
	if _, err := s.SignUp(context.Background(), "Bia", false); !errors.Is(err, ErrNotNow) {
		t.Fatalf("no account was made and the sign up said %v", err)
	}
	// Ana's name is taken by her account; her mark is not Bia's.
	var n int
	s.index.DB().QueryRow(`SELECT count(*) FROM accounts`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d accounts", n)
	}
}

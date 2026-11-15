package machines

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/permissions"
	"github.com/earthler/earthler/core/scribe"
)

// What is counted travels between machines as a form, and comes back as a
// number or as one of these words, which say which refusal it was.
var refusals = map[string]error{
	"empty":       accounts.ErrNameEmpty,
	"long":        accounts.ErrNameLong,
	"taken":       accounts.ErrNameTaken,
	"invalid":     accounts.ErrNameInvalid,
	"not-allowed": accounts.ErrNotAllowed,
	"no-account":  accounts.ErrNoSuchAccount,
	"bad-role":    accounts.ErrBadRole,
	"last-admin":  accounts.ErrLastAdministrator,
}

// refuse answers a counted act that was refused, or that failed.
func refuse(w http.ResponseWriter, err error) {
	for word, known := range refusals {
		if errors.Is(err, known) {
			http.Error(w, word, http.StatusConflict)
			return
		}
	}
	if errors.Is(err, scribe.ErrStopped) {
		http.Error(w, "stopped", http.StatusServiceUnavailable)
		return
	}
	log.Printf("machines: counting: %v", err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}

// countSignUp takes a name for another machine's sign up screen.
func (s *Server) countSignUp(w http.ResponseWriter, r *http.Request) {
	if s.Self != Counter {
		http.Error(w, "this machine does not count", http.StatusConflict)
		return
	}
	device, err := s.Accounts.Register(r.Context(), r.FormValue("name"))
	if err != nil {
		refuse(w, err)
		return
	}
	io.WriteString(w, strconv.FormatUint(device, 10))
}

// countRole changes a role for another machine's list of residents. The one
// who acts is the resident behind the device, with the role this machine
// knows.
func (s *Server) countRole(w http.ResponseWriter, r *http.Request) {
	if s.Self != Counter {
		http.Error(w, "this machine does not count", http.StatusConflict)
		return
	}
	device, err1 := strconv.ParseUint(r.FormValue("device"), 10, 63)
	target, err2 := strconv.ParseUint(r.FormValue("target"), 10, 63)
	if err1 != nil || err2 != nil {
		http.Error(w, "device and target must be ids", http.StatusBadRequest)
		return
	}
	if err := s.Accounts.ChangeRoleFor(r.Context(), device, target, permissions.Role(r.FormValue("role"))); err != nil {
		refuse(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Remote is the machine that counts, seen from a machine that does not. A
// machine that cannot be reached, or that does not answer in time, is
// accounts.ErrNotNow: nothing was done.
type Remote struct {
	Peers  *Peers
	Token  string
	Client *http.Client
}

// countWait is how long a machine waits for the machine that counts.
var countWait = 5 * time.Second

func (c *Remote) post(ctx context.Context, path string, form url.Values) (string, error) {
	address, ok := c.Peers.Get(Counter)
	if !ok {
		return "", accounts.ErrNotNow
	}
	ctx, cancel := context.WithTimeout(ctx, countWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(address, "/")+path, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.Client.Do(req)
	if err != nil {
		log.Printf("machines: the machine that counts: %v", err)
		return "", accounts.ErrNotNow
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	body := strings.TrimSpace(string(raw))
	switch {
	case res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNoContent:
		return body, nil
	case res.StatusCode == http.StatusConflict && refusals[body] != nil:
		return "", refusals[body]
	default:
		log.Printf("machines: the machine that counts answered %d: %s", res.StatusCode, body)
		return "", accounts.ErrNotNow
	}
}

// SignUp asks the machine that counts to take a name.
func (c *Remote) SignUp(ctx context.Context, name string) (uint64, error) {
	body, err := c.post(ctx, "/machines/count/sign-up", url.Values{"name": {name}})
	if err != nil {
		return 0, err
	}
	device, err := strconv.ParseUint(body, 10, 63)
	if err != nil {
		return 0, accounts.ErrNotNow
	}
	return device, nil
}

// ChangeRole asks the machine that counts to change a role.
func (c *Remote) ChangeRole(ctx context.Context, actor accounts.Session, target uint64, role permissions.Role) error {
	_, err := c.post(ctx, "/machines/count/role", url.Values{
		"device": {strconv.FormatUint(actor.Device, 10)},
		"target": {strconv.FormatUint(target, 10)},
		"role":   {string(role)},
	})
	return err
}

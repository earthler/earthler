package machines

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/earthler/earthler/core/durable"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/replica"
)

// Welcome is what a machine receives when it joins: its number, the token it
// shows the others, the addresses of the others that the machine that took
// it in knows, and the community's secrets, so it can serve the residents
// as an image of the others.
type Welcome struct {
	Machine uint64            `json:"machine"`
	Token   string            `json:"token"`
	Peers   map[uint64]string `json:"peers"`
	Secrets map[string][]byte `json:"secrets"` // by path inside the secrets folder
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

// join takes a new machine in. The invitation code proves that a person who
// runs this machine asked for it; the connection, which only the community
// root vouches for, keeps the secrets from anyone else on the way.
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	if s.Self != Counter {
		http.Error(w, "only the machine that counts takes new machines in", http.StatusConflict)
		return
	}
	reach := r.FormValue("url")
	if CheckAddress(reach) != nil {
		http.Error(w, "the address of the new machine is not an https address", http.StatusBadRequest)
		return
	}
	switch err := s.Invites.Spend(r.FormValue("code"), who(r), s.now()); {
	case errors.Is(err, replica.ErrTooMany):
		http.Error(w, "too many tries", http.StatusTooManyRequests)
		return
	case errors.Is(err, replica.ErrBadCode):
		http.Error(w, "the code is wrong, was used or has expired", http.StatusForbidden)
		return
	case err != nil:
		log.Printf("machines: joining: %v", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}
	welcome, err := s.add(r.Context(), tidyLabel(r.FormValue("label")), reach)
	if err != nil {
		log.Printf("machines: joining: %v", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}
	writeJSON(w, welcome)
}

// add writes the new machine in the journal, and this machine too the first
// time, keeps the new machine's address and gathers what it receives. What
// can fail is done before the journal is written, so a failure leaves no
// machine whose token nobody holds, except an answer lost on the way.
func (s *Server) add(ctx context.Context, label, reach string) (Welcome, error) {
	s.joining.Lock()
	defer s.joining.Unlock()
	secrets, err := readSecrets(s.Secrets)
	if err != nil {
		return Welcome{}, err
	}
	all, err := List(ctx, s.Index)
	if err != nil {
		return Welcome{}, err
	}
	var contents [][]byte
	listed, next := false, uint64(1)
	for _, m := range all {
		listed = listed || m.Number == s.Self
		next = max(next, m.Number+1)
	}
	if !listed {
		// The first invitation: this machine gets the token it will show
		// the others, kept before it is written, so a cut in between never
		// leaves a token in the journal that nobody holds.
		id, err := LoadIdentity(s.Secrets)
		if err != nil {
			return Welcome{}, err
		}
		if id.Token == "" {
			if id.Token, err = newToken(); err != nil {
				return Welcome{}, err
			}
			if err := SaveIdentity(s.Secrets, id); err != nil {
				return Welcome{}, err
			}
		}
		c, err := event.New(event.MachineAdded, 0, 0, s.now(), event.MachineAddedData{Machine: s.Self, Label: tidyLabel(s.Label), Token: digest(id.Token)})
		if err != nil {
			return Welcome{}, err
		}
		contents = append(contents, c)
	}
	if next > event.MaxMachine {
		return Welcome{}, errors.New("the community has every machine it can have")
	}
	token, err := newToken()
	if err != nil {
		return Welcome{}, err
	}
	c, err := event.New(event.MachineAdded, 0, 0, s.now(), event.MachineAddedData{Machine: next, Label: label, Token: digest(token)})
	if err != nil {
		return Welcome{}, err
	}
	if _, err := s.Scribe.Write(ctx, append(contents, c)...); err != nil {
		return Welcome{}, err
	}
	if _, err := s.Peers.Set(next, reach); err != nil {
		return Welcome{}, err
	}
	if err := s.Joined(next); err != nil {
		log.Printf("machines: machine %d joined, but keeping its journal failed: %v", next, err)
	}
	peers := s.Peers.All()
	delete(peers, next)
	return Welcome{Machine: next, Token: token, Peers: peers, Secrets: secrets}, nil
}

// readSecrets gathers every file of the secrets folder but the identity of
// this machine.
func readSecrets(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == IdentityFile {
			return err
		}
		raw, err := os.ReadFile(path)
		out[filepath.ToSlash(rel)] = raw
		return err
	})
	return out, err
}

// JoinOptions is what a person gives to bring a new machine in.
type JoinOptions struct {
	URL         string // the secure address of the machine that counts
	PlainURL    string // the address of its welcome page, where the root is fetched
	Code        string // the invitation code it printed
	Fingerprint string // the fingerprint of the community root it printed
	Label       string // a name for this machine
	Reach       string // the secure address at which the others reach this machine
	DataDir     string // an empty data folder for this machine
}

// Join brings a new machine into a community: it trusts the community root
// only if its fingerprint is the one the person read, trades the invitation
// code for a number, a token and the secrets, and writes them in the data
// folder, which must not belong to a community yet, and reads the journal of
// the machine that counts up to that moment. The identity of the machine is
// written last, so a join cut halfway can be tried again with a new code.
func Join(ctx context.Context, o JoinOptions) (Welcome, error) {
	secrets := filepath.Join(o.DataDir, "secrets")
	if _, err := os.Stat(filepath.Join(secrets, IdentityFile)); err == nil {
		return Welcome{}, fmt.Errorf("machines: %s already belongs to a community", o.DataDir)
	}
	if entries, err := os.ReadDir(filepath.Join(o.DataDir, "journal")); err == nil && len(entries) > 0 {
		return Welcome{}, fmt.Errorf("machines: %s already has a journal", o.DataDir)
	}
	if err := CheckAddress(o.Reach); err != nil {
		return Welcome{}, err
	}
	root, err := replica.FetchRoot(ctx, o.PlainURL)
	if err != nil {
		return Welcome{}, err
	}
	if replica.Fingerprint(root) != strings.ToUpper(strings.NewReplacer(" ", "", ":", "", "-", "").Replace(o.Fingerprint)) {
		return Welcome{}, errors.New("machines: the certificate at that address is not the one with the fingerprint you gave")
	}
	client, err := replica.Pinned(root)
	if err != nil {
		return Welcome{}, err
	}
	form := url.Values{"code": {o.Code}, "label": {o.Label}, "url": {o.Reach}}
	res, err := client.PostForm(strings.TrimSuffix(o.URL, "/")+"/machines/join", form)
	if err != nil {
		return Welcome{}, fmt.Errorf("machines: the machine that counts could not be reached: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return Welcome{}, fmt.Errorf("machines: joining was refused (%d): %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var w Welcome
	if err := json.Unmarshal(body, &w); err != nil || w.Machine == 0 || w.Machine > event.MaxMachine || w.Token == "" {
		return Welcome{}, errors.New("machines: the answer of the machine that counts is not a welcome")
	}
	for rel, raw := range w.Secrets {
		path := filepath.Join(secrets, filepath.FromSlash(rel))
		if rel == IdentityFile || !strings.HasPrefix(path, secrets+string(filepath.Separator)) {
			return Welcome{}, fmt.Errorf("machines: the welcome has a secret at %q", rel)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return Welcome{}, err
		}
		if err := durable.WriteFile(path, raw, 0o600); err != nil {
			return Welcome{}, err
		}
	}
	peers, err := OpenPeers(PeersPath(o.DataDir))
	if err != nil {
		return Welcome{}, err
	}
	if w.Peers == nil {
		w.Peers = map[uint64]string{}
	}
	w.Peers[Counter] = strings.TrimSuffix(o.URL, "/")
	for n, a := range w.Peers {
		if n != w.Machine {
			if _, err := peers.Set(n, a); err != nil {
				return Welcome{}, err
			}
		}
	}
	copy, err := journal.Open(filepath.Join(o.DataDir, "journals", strconv.FormatUint(Counter, 10)), journal.Options{})
	if err != nil {
		return Welcome{}, err
	}
	defer copy.Close()
	f := &Follower{Peer: Counter, Peers: peers, Token: w.Token, Client: client, Copy: copy}
	if _, err := f.CatchUp(ctx); err != nil {
		return Welcome{}, fmt.Errorf("machines: the journal of the machine that counts could not be read; join again with a new code: %w", err)
	}
	// The identity goes last: a folder with it is a machine of the community.
	return w, SaveIdentity(secrets, Identity{Number: w.Machine, Token: w.Token})
}

// PeersPath is where a data folder keeps the addresses of the other machines.
func PeersPath(dataDir string) string { return filepath.Join(dataDir, "machines", "peers.json") }

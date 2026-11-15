package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/machines"
	"github.com/earthler/earthler/core/replica"
)

// many is what a machine keeps to live with the other machines of its
// community: the copies of their journals, their addresses, and one
// follower for each.
type many struct {
	mu        sync.Mutex
	kept      map[uint64]*journal.Journal // the copies of the other machines' journals
	following map[uint64]bool
	keeping   sync.Mutex // one copy is opened at a time, so none is opened twice

	peers    *machines.Peers
	client   *http.Client // trusts the community root and nothing else
	machines *machines.Server

	ctx   context.Context
	quit  context.CancelFunc
	group sync.WaitGroup
}

// gossipEvery is how often a machine asks the others which addresses they
// know.
var gossipEvery = 30 * time.Second

func (a *app) self() uint64 { return a.identity.Number }

// openJournals opens this machine's own journal and the copies it keeps,
// one folder per machine under journals.
func (a *app) openJournals() error {
	var err error
	if a.journal, err = journal.Open(filepath.Join(a.dataDir, "journal"), journal.Options{}); err != nil {
		return err
	}
	dirs, err := os.ReadDir(filepath.Join(a.dataDir, "journals"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, d := range dirs {
		n, err := strconv.ParseUint(d.Name(), 10, 64)
		if err != nil || !d.IsDir() || n == a.self() || n > event.MaxMachine || strconv.FormatUint(n, 10) != d.Name() {
			continue
		}
		j, err := journal.Open(filepath.Join(a.dataDir, "journals", d.Name()), journal.Options{})
		if err != nil {
			return err
		}
		a.kept[n] = j
	}
	return nil
}

// holding returns every journal the machine holds, its own among them.
func (a *app) holding() map[uint64]*journal.Journal {
	a.mu.Lock()
	defer a.mu.Unlock()
	all := map[uint64]*journal.Journal{}
	for n, j := range a.kept {
		all[n] = j
	}
	if a.journal != nil {
		all[a.self()] = a.journal
	}
	return all
}

// startMachines serves the other machines, follows each of them, and sends
// what is counted to the machine that counts when that is another one.
func (a *app) startMachines(cfg config, mux *http.ServeMux, residents *accounts.Service) error {
	var err error
	secrets := filepath.Join(a.dataDir, "secrets")
	if a.peers, err = machines.OpenPeers(machines.PeersPath(a.dataDir)); err != nil {
		return err
	}
	if a.client, err = replica.Pinned(a.chain.Root()); err != nil {
		return err
	}
	invites, err := replica.OpenRegistry(filepath.Join(a.dataDir, "machines"))
	if err != nil {
		return err
	}
	label := cfg.label
	if label == "" {
		label = "machine " + strconv.FormatUint(a.self(), 10)
	}
	a.machines = &machines.Server{
		Self: a.self(), Label: label, Secrets: secrets, Index: a.byName[machines.Area.Name],
		Scribe: a.scribe, Own: a.journal, Invites: invites, Peers: a.peers, Accounts: residents,
		Joined: a.keep,
	}
	a.machines.Routes(mux)
	if a.self() != machines.Counter {
		residents.CountAt(&machines.Remote{Peers: a.peers, Token: a.identity.Token, Client: a.client})
	}

	a.following = map[uint64]bool{}
	a.ctx, a.quit = context.WithCancel(context.Background())
	// A machine that joins while this one runs is found in the journal of
	// the machine that counts.
	a.scribe.Listen(a.notice)
	known := map[uint64]bool{}
	for n := range a.holding() {
		known[n] = true
	}
	for n := range a.peers.All() {
		known[n] = true
	}
	all, err := machines.List(context.Background(), a.byName[machines.Area.Name])
	if err != nil {
		return err
	}
	for _, m := range all {
		known[m.Number] = true
	}
	for n := range known {
		if err := a.keep(n); err != nil {
			return err
		}
	}
	a.group.Add(1)
	go func() {
		defer a.group.Done()
		a.gossip()
	}()
	return nil
}

// keep starts keeping a copy of machine n's journal and following it, once.
func (a *app) keep(n uint64) error {
	if n == a.self() || n > event.MaxMachine {
		return nil
	}
	a.keeping.Lock()
	defer a.keeping.Unlock()
	a.mu.Lock()
	j := a.kept[n]
	a.mu.Unlock()
	if j == nil {
		var err error
		if j, err = journal.Open(filepath.Join(a.dataDir, "journals", strconv.FormatUint(n, 10)), journal.Options{}); err != nil {
			return err
		}
		if err := a.scribe.Keep(context.Background(), n, j); err != nil {
			j.Close()
			return err
		}
		a.mu.Lock()
		a.kept[n] = j
		a.mu.Unlock()
	}
	a.follow(n, j)
	return nil
}

// follow starts the follower of machine n, once. A machine that has no token
// yet, the founding machine before its first invitation, follows nobody.
func (a *app) follow(n uint64, copy *journal.Journal) {
	id, err := machines.LoadIdentity(filepath.Join(a.dataDir, "secrets"))
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil || id.Token == "" || a.following[n] || a.ctx == nil || a.ctx.Err() != nil {
		return
	}
	a.following[n] = true
	f := &machines.Follower{Peer: n, Peers: a.peers, Token: id.Token, Client: a.client, Copy: copy, Scribe: a.scribe, Logf: log.Printf}
	a.group.Add(1)
	go func() {
		defer a.group.Done()
		f.Run(a.ctx)
	}()
}

// notice is told about every batch; a machine added by the machine that
// counts is a machine to keep and follow. It cannot ask the scribe for
// anything itself, so it leaves that to another flow.
func (a *app) notice(entries []index.Entry) {
	for _, e := range entries {
		if e.Machine != machines.Counter {
			continue
		}
		ev, err := event.Decode(e.Content)
		var d event.MachineAddedData
		if err != nil || ev.Type != event.MachineAdded || ev.Read(&d) != nil || d.Machine == a.self() {
			continue
		}
		go func(n uint64) {
			if err := a.keep(n); err != nil {
				log.Printf("machines: keeping machine %d: %v", n, err)
			}
		}(d.Machine)
	}
}

// gossip asks every machine it knows, now and then, which addresses it
// knows, so that every machine learns every other and can follow it even
// when the machine that counts is away.
func (a *app) gossip() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(gossipEvery):
		}
		id, err := machines.LoadIdentity(filepath.Join(a.dataDir, "secrets"))
		if err != nil || id.Token == "" {
			continue
		}
		for _, address := range a.peers.All() {
			for n, other := range a.askPeers(address, id.Token) {
				if n == a.self() {
					continue
				}
				if changed, err := a.peers.Set(n, other); err == nil && changed {
					a.keep(n)
				}
			}
		}
	}
}

func (a *app) askPeers(address, token string) map[uint64]string {
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(address, "/")+"/machines/peers", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := a.client.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	var got map[uint64]string
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&got) != nil {
		return nil
	}
	return got
}

// stopFollowing ends the followers and the gossip, and waits for them.
func (a *app) stopFollowing() {
	a.mu.Lock()
	quit := a.quit
	a.mu.Unlock()
	if quit != nil {
		quit()
	}
	a.group.Wait()
}

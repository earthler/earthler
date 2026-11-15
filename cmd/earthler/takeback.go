package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/machines"
	"github.com/earthler/earthler/core/replica"
)

// gossip asks every machine it knows, now and then, which addresses it
// knows, so that every machine learns every other and can follow it even
// when the machine that counts is away. It only learns machines it did not
// know: an address it has is the one it reached the machine at, and another
// machine may reach the same machine at another address.
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
				if _, known := a.peers.Get(n); known || n == a.self() {
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

// takeBack asks the other machines, before this machine writes anything, for
// the end of its own journal that they hold and it does not: what a machine
// restored from an older copy lost. Without it, the entries it wrote next
// would take numbers the others already hold with other contents, and the
// community would split. A machine that cannot be reached is skipped: the
// machine starts anyway, as a machine alone does.
func (a *app) takeBack() error {
	secrets := filepath.Join(a.dataDir, "secrets")
	if a.identity.Token == "" {
		return nil
	}
	chain, err := certificate.Load(filepath.Join(secrets, "certificate"))
	if err != nil {
		return nil
	}
	client, err := replica.Pinned(chain.Root())
	if err != nil {
		return err
	}
	peers, err := machines.OpenPeers(machines.PeersPath(a.dataDir))
	if err != nil {
		return err
	}
	for n := range peers.All() {
		before, _ := a.journal.Last()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		f := &machines.Follower{Peer: n, Peers: peers, Token: a.identity.Token, Client: client, Copy: a.journal, Back: true, Self: a.self()}
		got, err := f.CatchUp(ctx)
		cancel()
		switch {
		case got > 0:
			log.Printf("machine %d held %d entries of this machine's journal, after entry %d, that it had lost; they are back", n, got, before)
		case errors.Is(err, journal.ErrNotNext):
			log.Printf("machine %d holds a journal of this machine that is not this one: %v", n, err)
		}
	}
	return nil
}

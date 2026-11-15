package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/erasure"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/machines"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/replica"
	"github.com/earthler/earthler/core/scribe"
	"github.com/earthler/earthler/screens/auditlog"
	"github.com/earthler/earthler/screens/chat"
	people "github.com/earthler/earthler/screens/residents"
	"github.com/earthler/earthler/screens/search"
	"github.com/earthler/earthler/screens/signup"
	"github.com/earthler/earthler/screens/welcome"
)

// config is what the server needs to start.
type config struct {
	dataDir   string // where the journal, the indexes and the secrets live
	community string // name given to a community that is being born
	short     string // short name for the address <short>.internal, fixed at birth
	lang      string // language of the screens
	httpsPort int    // where the secure site answers, for the links of the welcome page
	copyDir   string // where the night's copy goes, or empty for no copies yet
	label     string // a name for this machine, for the other machines
}

var validShort = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// app is the whole server: the journals, the indexes derived from them, the
// scribe that writes, the screens, and what it does with the other machines.
type app struct {
	identity machines.Identity
	journal  *journal.Journal // this machine's own
	indexes  []*index.Index
	scribe   *scribe.Scribe
	chain    *certificate.Chain
	name     string
	byName   map[string]*index.Index
	dataDir  string
	copyDir  string

	handler http.Handler // the system, served only over HTTPS
	welcome http.Handler // the welcome page and the certificate, served over plain HTTP
	stop    context.CancelFunc
	nights  sync.WaitGroup // the night's work, which must end before the files close
	chat    *chat.Screen

	many // the other machines
}

// nightHour is when the night's work runs, in the machine's local time.
const nightHour = 3

// open starts everything from the data folder. The indexes are brought level
// with the journal before any screen is served, which is also how the system
// comes back after a power cut.
func open(cfg config) (*app, error) {
	a := &app{dataDir: cfg.dataDir, copyDir: cfg.copyDir}
	a.kept = map[uint64]*journal.Journal{}
	ok := false
	defer func() {
		if !ok {
			a.Close()
		}
	}()

	var err error
	if a.identity, err = machines.LoadIdentity(filepath.Join(cfg.dataDir, "secrets")); err != nil {
		return nil, err
	}
	if err := a.openJournals(); err != nil {
		return nil, err
	}
	if err := a.takeBack(); err != nil {
		return nil, err
	}
	areas := []index.Area{accounts.Area, chat.Area, search.Area, auditlog.Area, erasure.Area(a.self()), machines.Area}
	byName := map[string]*index.Index{}
	for _, area := range areas {
		ix, err := index.Open(filepath.Join(cfg.dataDir, "indexes"), area)
		if err != nil {
			return nil, err
		}
		a.indexes = append(a.indexes, ix)
		byName[area.Name] = ix
	}
	a.byName = byName
	held := a.holding()
	delete(held, a.self())
	if a.scribe, err = scribe.Open(scribe.Journals{Self: a.self(), Own: a.journal, Kept: held}, a.indexes...); err != nil {
		return nil, err
	}
	if a.name, err = a.community(cfg); err != nil {
		return nil, err
	}
	// What was deleted before the last stop is erased before any screen is
	// served, so a restored copy never serves a text that was deleted.
	if _, err := a.erase(context.Background(), time.Now()); err != nil {
		return nil, err
	}
	var ctx context.Context
	ctx, a.stop = context.WithCancel(context.Background())
	a.nights.Add(1)
	go func() {
		defer a.nights.Done()
		a.night(ctx)
	}()

	key, err := accounts.LoadKey(filepath.Join(cfg.dataDir, "secrets"))
	if err != nil {
		return nil, err
	}
	// The copy key is born with the community, so the first copy can be
	// read back with the key written down on the first day.
	copyKey, err := copies.LoadKey(filepath.Join(cfg.dataDir, "secrets"))
	if err != nil {
		return nil, err
	}
	pages, err := page.New(cfg.lang, a.name)
	if err != nil {
		return nil, err
	}
	residents := accounts.New(byName[accounts.Area.Name], a.scribe, key)

	// The chain is created once. Its name can never change afterwards,
	// because every device has it written in the certificate it installed.
	if !validShort.MatchString(cfg.short) {
		return nil, fmt.Errorf("the short name %q must use lowercase letters, digits and hyphens", cfg.short)
	}
	a.chain, _, err = certificate.Open(filepath.Join(cfg.dataDir, "secrets", "certificate"), cfg.short+".internal", a.name, time.Now())
	if err != nil {
		return nil, err
	}
	plain := http.NewServeMux()
	plain.Handle("GET /static/", page.Static())
	welcome.New(a.chain, pages, cfg.httpsPort).Routes(plain)
	a.welcome = pages.Guard(plain)

	mux := http.NewServeMux()
	mux.Handle("GET /static/", page.Static())
	mux.HandleFunc("GET /probe.gif", welcome.Probe)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if _, signedIn := residents.Session(r); signedIn {
			http.Redirect(w, r, "/chat", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/sign-up", http.StatusSeeOther)
	})
	signup.New(residents, pages).Routes(mux)
	a.chat = chat.New(byName[chat.Area.Name], a.scribe, pages)
	a.chat.Routes(mux, residents.Require)
	search.New(byName[search.Area.Name], pages).Routes(mux, residents.Require)
	auditlog.New(byName[auditlog.Area.Name], pages).Routes(mux, residents.Require)
	people.New(residents, pages).Routes(mux, residents.Require)

	// A second machine pulls the encrypted copy from here, with a token of
	// its own that a pairing code gave it.
	pairings, err := replica.OpenRegistry(filepath.Join(cfg.dataDir, "replication"))
	if err != nil {
		return nil, err
	}
	(&replica.Server{
		Registry: pairings, Journal: a.journal, DataDir: cfg.dataDir, Key: copyKey,
		Out: filepath.Join(cfg.dataDir, "replication", "out"), MinGap: 5 * time.Second,
	}).Routes(mux)
	if err := a.startMachines(cfg, mux, residents); err != nil {
		return nil, err
	}
	a.handler = pages.Guard(mux)

	ok = true
	return a, nil
}

// Hush ends the connections that pages keep open, which never go quiet by
// themselves. A server that is stopping calls it before it waits for the
// requests that are still running.
func (a *app) Hush() {
	if a.chat != nil {
		a.chat.Close()
	}
	if a.machines != nil {
		a.machines.Close()
	}
}

// Close stops the night's work, the scribe and the files. The night's work
// finishes what it is doing first: closing the indexes under an erasure or a
// copy would leave it half done.
func (a *app) Close() {
	a.Hush()
	a.stopFollowing()
	if a.stop != nil {
		a.stop()
	}
	a.nights.Wait()
	if a.scribe != nil {
		a.scribe.Close()
	}
	for _, ix := range a.indexes {
		ix.Close()
	}
	for _, j := range a.holding() {
		j.Close()
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/certificate"
	"github.com/earthler/earthler/core/erasure"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/scribe"
	"github.com/earthler/earthler/screens/chat"
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
}

var validShort = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// app is the whole server: the journal, the indexes derived from it, the
// scribe that writes, and the screens.
type app struct {
	journal *journal.Journal
	indexes []*index.Index
	scribe  *scribe.Scribe
	chain   *certificate.Chain
	name    string
	byName  map[string]*index.Index

	handler http.Handler // the system, served only over HTTPS
	welcome http.Handler // the welcome page and the certificate, served over plain HTTP
	stop    context.CancelFunc
}

// nightHour is when the night's work runs, in the machine's local time.
const nightHour = 3

// open starts everything from the data folder. The indexes are brought level
// with the journal before any screen is served, which is also how the system
// comes back after a power cut.
func open(cfg config) (*app, error) {
	a := &app{}
	ok := false
	defer func() {
		if !ok {
			a.Close()
		}
	}()

	var err error
	if a.journal, err = journal.Open(filepath.Join(cfg.dataDir, "journal"), journal.Options{}); err != nil {
		return nil, err
	}
	areas := []index.Area{accounts.Area, chat.Area, search.Area, erasure.Area}
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
	if a.scribe, err = scribe.Start(a.journal, a.indexes...); err != nil {
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
	go a.nights(ctx)

	key, err := accounts.LoadKey(filepath.Join(cfg.dataDir, "secrets"))
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
	a.welcome = plain

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
	chat.New(byName[chat.Area.Name], a.scribe, pages).Routes(mux, residents.Require)
	search.New(byName[search.Area.Name], pages).Routes(mux, residents.Require)
	a.handler = mux

	ok = true
	return a, nil
}

var errFound = errors.New("found")

// community returns the name of the community. In an empty journal it first
// writes the birth of the community, which is always entry 1, and the open
// channel everyone starts with.
func (a *app) community(cfg config) (string, error) {
	if last, _ := a.journal.Last(); last == 0 {
		texts, err := page.New(cfg.lang, cfg.community)
		if err != nil {
			return "", err
		}
		now := time.Now()
		born, err := event.New(event.CommunityCreated, 0, 0, now, event.CommunityCreatedData{Name: cfg.community})
		if err != nil {
			return "", err
		}
		channel, err := event.New(event.ChannelCreated, 0, 0, now, event.ChannelCreatedData{Name: texts.T("chat.general")})
		if err != nil {
			return "", err
		}
		if _, err := a.scribe.Write(context.Background(), born, channel); err != nil {
			return "", err
		}
	}
	var name string
	err := a.journal.Read(1, func(e journal.Entry) error {
		ev, err := event.Decode(e.Content)
		if err != nil {
			return err
		}
		var d event.CommunityCreatedData
		if ev.Type != event.CommunityCreated || ev.Read(&d) != nil {
			return errors.New("entry 1 of the journal is not the birth of a community")
		}
		name = d.Name
		return errFound
	})
	if !errors.Is(err, errFound) {
		if err == nil {
			err = errors.New("entry 1 of the journal was erased")
		}
		return "", err
	}
	return name, nil
}

// erase runs the physical erasure of deleted content.
func (a *app) erase(ctx context.Context, now time.Time) (int, error) {
	n, err := erasure.Run(ctx, a.journal, a.byName[erasure.Area.Name], a.scribe, now)
	if n > 0 {
		log.Printf("erased the content of %d entries", n)
	}
	return n, err
}

// nights runs the night's work: the erasure, and later the copies.
func (a *app) nights(ctx context.Context) {
	for {
		wait := time.NewTimer(time.Until(erasure.NextNight(time.Now(), nightHour)))
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
		if _, err := a.erase(ctx, time.Now()); err != nil {
			log.Printf("erasure: %v", err)
		}
	}
}

// Close stops the scribe and closes the files.
func (a *app) Close() {
	if a.stop != nil {
		a.stop()
	}
	if a.scribe != nil {
		a.scribe.Close()
	}
	for _, ix := range a.indexes {
		ix.Close()
	}
	if a.journal != nil {
		a.journal.Close()
	}
}

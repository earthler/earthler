package main

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/page"
	"github.com/earthler/earthler/core/scribe"
	"github.com/earthler/earthler/screens/chat"
	"github.com/earthler/earthler/screens/search"
	"github.com/earthler/earthler/screens/signup"
)

// config is what the server needs to start.
type config struct {
	dataDir   string // where the journal, the indexes and the secrets live
	community string // name given to a community that is being born
	lang      string // language of the screens
}

// app is the whole server: the journal, the indexes derived from it, the
// scribe that writes, and the screens.
type app struct {
	journal *journal.Journal
	indexes []*index.Index
	scribe  *scribe.Scribe
	handler http.Handler
	name    string
}

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
	areas := []index.Area{accounts.Area, chat.Area, search.Area}
	byName := map[string]*index.Index{}
	for _, area := range areas {
		ix, err := index.Open(filepath.Join(cfg.dataDir, "indexes"), area)
		if err != nil {
			return nil, err
		}
		a.indexes = append(a.indexes, ix)
		byName[area.Name] = ix
	}
	if a.scribe, err = scribe.Start(a.journal, a.indexes...); err != nil {
		return nil, err
	}
	if a.name, err = a.community(cfg); err != nil {
		return nil, err
	}

	key, err := accounts.LoadKey(filepath.Join(cfg.dataDir, "secrets"))
	if err != nil {
		return nil, err
	}
	pages, err := page.New(cfg.lang, a.name)
	if err != nil {
		return nil, err
	}
	residents := accounts.New(byName[accounts.Area.Name], a.scribe, key)

	mux := http.NewServeMux()
	mux.Handle("GET /static/", page.Static())
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

// Close stops the scribe and closes the files.
func (a *app) Close() {
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

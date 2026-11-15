package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/earthler/earthler/core/erasure"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/machines"
	"github.com/earthler/earthler/core/page"
)

var errFound = errors.New("found")

// community returns the name of the community, from entry 1 of the journal
// of the machine that founded it. On that machine, in an empty journal, it
// first writes the birth of the community and the open channel everyone
// starts with. Another machine holds the copy it received when it joined.
func (a *app) community(cfg config) (string, error) {
	founder := a.journal
	if a.self() != machines.Counter {
		founder = a.holding()[machines.Counter]
		if founder == nil {
			return "", errors.New("this machine holds no copy of the journal of the machine that founded the community: join again")
		}
	} else if last, _ := a.journal.Last(); last == 0 {
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
	err := founder.Read(1, func(e journal.Entry) error {
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
	n, err := erasure.Run(ctx, a.holding(), a.byName["erasure"], a.scribe, now)
	if n > 0 {
		log.Printf("erased the content of %d entries", n)
	}
	return n, err
}

// night runs the night's work: first the erasure, then the copy, so a
// copy never carries a text that was deleted before it.
func (a *app) night(ctx context.Context) {
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
		if a.copyDir != "" {
			if err := makeCopy(a.dataDir, a.copyDir); err != nil {
				log.Printf("copy: %v", err)
			}
		}
	}
}

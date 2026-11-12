package replica

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/earthler/earthler/core/copies"
)

// sweep removes what the copy no longer has: old versions of files, which is
// also how a deletion reaches this machine, and leftovers of a cut pull.
func (f *Follower) sweep(keep map[string]File) error {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == stateFile {
			continue
		}
		if _, ok := keep[name]; ok {
			continue
		}
		if copies.IsOurs(strings.TrimSuffix(name, ".tmp")) {
			if err := os.Remove(filepath.Join(f.dir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// setAside moves what the folder holds into a folder of its own, because the
// main machine's history is not the one it was following: it was restored
// from an older copy, or it is another community. Nothing is deleted, so a
// person can still look at it, and the pull starts again from nothing.
func (f *Follower) setAside() error {
	stamp := time.Now().UTC().Format("20060102-150405")
	aside := filepath.Join(f.dir, divergedDir, stamp)
	if err := os.MkdirAll(aside, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && copies.IsOurs(strings.TrimSuffix(e.Name(), ".tmp")) {
			if err := os.Rename(filepath.Join(f.dir, e.Name()), filepath.Join(aside, e.Name())); err != nil {
				return err
			}
		}
	}
	f.state.Held, f.state.Last, f.state.Seal = map[string]File{}, 0, ""
	return f.save()
}

// Run pulls every interval until the context ends, reporting each pull
// through log. A main machine that cannot be reached is tried again; a token
// that was revoked ends the run, because trying again would never work.
func (f *Follower) Run(ctx context.Context, every time.Duration, log func(format string, args ...any)) error {
	for {
		done, err := f.Pull(ctx)
		switch {
		case errors.Is(err, ErrRevoked):
			return err
		case err != nil && ctx.Err() == nil:
			log("replica: %v; trying again in %s", err, every)
		case err == nil && done.Downloaded > 0:
			log("replica: %s", done)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}

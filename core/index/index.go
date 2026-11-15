// Package index keeps the SQLite files that screens and search read. Every
// index is derived from the journals: it can be deleted at any time and
// rebuilt from the entries.
//
// A machine holds its own journal and a copy of each other machine's, and an
// index applies the entries of all of them. Each journal is applied in its
// own order, but two journals can arrive in any order relative to each other,
// so an Apply function has to reach the same tables whichever came first.
package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // the SQLite driver, in pure Go

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/journal"
)

// Area describes one index file: its tables and how each entry changes them.
type Area struct {
	// Name is the file name without extension, such as "chat".
	Name string

	// Schema creates the tables. Changing it makes the index rebuild itself
	// from the journals the next time it is opened.
	Schema string

	// Apply changes the index for one entry. It is called for every entry,
	// inside a transaction, and must ignore the entries it does not care
	// about. It never receives an erased entry.
	Apply func(tx *sql.Tx, e Entry) error
}

// Entry is one entry as an index receives it: the line of a journal, and
// which machine's journal it is.
type Entry struct {
	journal.Entry
	Machine uint64
	ID      uint64 // how the thing the entry creates is known, see event.ID
}

// Wrap turns the entries of one machine's journal into entries for an index.
func Wrap(machine uint64, entries []journal.Entry) []Entry {
	out := make([]Entry, len(entries))
	for i, e := range entries {
		out[i] = Entry{Entry: e, Machine: machine, ID: event.ID(machine, e.Number)}
	}
	return out
}

// Position is where an index is in one journal: the number and the seal of
// the last entry it applied.
type Position struct {
	Last uint64
	Seal journal.Seal
}

// Index is one open index file.
type Index struct {
	area Area
	path string
	db   *sql.DB

	mu sync.Mutex          // only the scribe changes at, but screens read it
	at map[uint64]Position // by machine
}

// Open opens the index of an area in dir. A missing file, or a file made
// with another schema, is created empty: Follow then fills it.
func Open(dir string, area Area) (*Index, error) {
	if area.Name == "" || area.Apply == nil {
		return nil, errors.New("index: an area needs a name and an Apply function")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	ix := &Index{area: area, path: filepath.Join(dir, area.Name+".db")}
	if err := ix.connect(); err != nil {
		// A file that SQLite cannot open is not an index we can use. Nothing
		// is lost by deleting it, because the journals have everything.
		if err := ix.reset(); err != nil {
			return nil, err
		}
		return ix, nil
	}
	ok, err := ix.load()
	if err != nil {
		ix.db.Close()
		return nil, err
	}
	if !ok {
		if err := ix.reset(); err != nil {
			return nil, err
		}
	}
	return ix, nil
}

func (ix *Index) connect() error {
	// WAL lets screens read while the scribe writes. NORMAL is enough because
	// the journals are the truth: an index that loses its last moments is
	// brought level again by Follow. secure_delete overwrites what is
	// deleted, so an erased text does not linger in the file.
	dsn := ix.path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=secure_delete(ON)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("index %s: %w", ix.area.Name, err)
	}
	ix.db = db
	return nil
}

// reset deletes the index file and creates it again, empty.
func (ix *Index) reset() error {
	if ix.db != nil {
		ix.db.Close()
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(ix.path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := ix.connect(); err != nil {
		return err
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(metaSchema + ix.area.Schema); err != nil {
		return fmt.Errorf("index %s: schema: %w", ix.area.Name, err)
	}
	ix.setAt(map[uint64]Position{})
	if err := ix.saveSchema(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (ix *Index) fingerprint() string {
	sum := sha256.Sum256([]byte(ix.area.Schema))
	return hex.EncodeToString(sum[:])
}

// Apply applies entries of one machine's journal to the index in one
// transaction: either all of them are applied, or none. The entries must
// follow the last one applied from that journal.
func (ix *Index) Apply(machine uint64, entries []journal.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	pos := ix.Position(machine)
	for _, e := range Wrap(machine, entries) {
		if e.Number != pos.Last+1 {
			return fmt.Errorf("index %s: is at entry %d of machine %d and received entry %d", ix.area.Name, pos.Last, machine, e.Number)
		}
		if !event.Valid(machine, e.Number) {
			return fmt.Errorf("index %s: entry %d of machine %d has no id", ix.area.Name, e.Number, machine)
		}
		if !e.Erased {
			if err := ix.area.Apply(tx, e); err != nil {
				return fmt.Errorf("index %s: entry %d of machine %d: %w", ix.area.Name, e.Number, machine, err)
			}
		}
		pos = Position{Last: e.Number, Seal: e.Seal()}
	}
	if err := savePosition(tx, machine, pos); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("index %s: %w", ix.area.Name, err)
	}
	ix.mu.Lock()
	ix.at[machine] = pos
	ix.mu.Unlock()
	return nil
}

// Last returns the number of the last entry applied from machine's journal.
func (ix *Index) Last(machine uint64) uint64 { return ix.Position(machine).Last }

// Position returns where the index is in machine's journal.
func (ix *Index) Position(machine uint64) Position {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.at[machine]
}

// Positions returns the last entry applied from each journal, by machine.
func (ix *Index) Positions() map[uint64]uint64 {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	out := make(map[uint64]uint64, len(ix.at))
	for m, p := range ix.at {
		out[m] = p.Last
	}
	return out
}

func (ix *Index) setAt(at map[uint64]Position) {
	ix.mu.Lock()
	ix.at = at
	ix.mu.Unlock()
}

// DB gives read access to the index. Screens query it; only Apply writes.
func (ix *Index) DB() *sql.DB { return ix.db }

// Close closes the file.
func (ix *Index) Close() error { return ix.db.Close() }

// Package index keeps the SQLite files that screens and search read. Every
// index is derived from the journal: it can be deleted at any time and
// rebuilt from the entries.
package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // the SQLite driver, in pure Go

	"github.com/earthler/earthler/core/journal"
)

// Area describes one index file: its tables and how each entry changes them.
type Area struct {
	// Name is the file name without extension, such as "chat".
	Name string

	// Schema creates the tables. Changing it makes the index rebuild itself
	// from the journal the next time it is opened.
	Schema string

	// Apply changes the index for one entry. It is called for every entry of
	// the journal, in order, inside a transaction, and must ignore the
	// entries it does not care about. It never receives an erased entry.
	Apply func(tx *sql.Tx, e journal.Entry) error
}

// Index is one open index file.
type Index struct {
	area Area
	path string
	db   *sql.DB
	last uint64       // number of the last entry applied
	seal journal.Seal // seal of that entry
}

const metaSchema = `CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID;`

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
		// is lost by deleting it, because the journal has everything.
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
	// the journal is the truth: an index that loses its last moments is
	// brought level again by Follow.
	dsn := ix.path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
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

// load reads the position of the index. It reports false when the file is
// new, was made with another schema, or cannot be read as an index.
func (ix *Index) load() (bool, error) {
	var schema, last, seal string
	err := ix.db.QueryRow(`SELECT
		(SELECT value FROM meta WHERE key = 'schema'),
		(SELECT value FROM meta WHERE key = 'last'),
		(SELECT value FROM meta WHERE key = 'seal')`).Scan(&schema, &last, &seal)
	if err != nil || schema != ix.fingerprint() {
		return false, nil // not an index we can use; it will be rebuilt
	}
	if _, err := fmt.Sscanf(last, "%d", &ix.last); err != nil {
		return false, nil
	}
	raw, err := hex.DecodeString(seal)
	if err != nil || len(raw) != len(ix.seal) {
		return false, nil
	}
	copy(ix.seal[:], raw)
	return true, nil
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
	ix.last, ix.seal = 0, journal.Seal{}
	if err := ix.savePosition(tx, true); err != nil {
		return err
	}
	return tx.Commit()
}

func (ix *Index) savePosition(tx *sql.Tx, withSchema bool) error {
	values := map[string]string{"last": fmt.Sprint(ix.last), "seal": ix.seal.String()}
	if withSchema {
		values["schema"] = ix.fingerprint()
	}
	for key, value := range values {
		if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (ix *Index) fingerprint() string {
	sum := sha256.Sum256([]byte(ix.area.Schema))
	return hex.EncodeToString(sum[:])
}

// Apply applies entries to the index in one transaction: either all of them
// are applied, or none. The entries must follow the last one applied.
func (ix *Index) Apply(entries []journal.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	last, seal := ix.last, ix.seal
	for _, e := range entries {
		if e.Number != last+1 {
			return fmt.Errorf("index %s: is at entry %d and received entry %d", ix.area.Name, last, e.Number)
		}
		if !e.Erased {
			if err := ix.area.Apply(tx, e); err != nil {
				return fmt.Errorf("index %s: entry %d: %w", ix.area.Name, e.Number, err)
			}
		}
		last, seal = e.Number, e.Seal()
	}
	before, beforeSeal := ix.last, ix.seal
	ix.last, ix.seal = last, seal
	if err := ix.savePosition(tx, false); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		ix.last, ix.seal = before, beforeSeal
		return fmt.Errorf("index %s: %w", ix.area.Name, err)
	}
	return nil
}

// errFound ends a journal read once the entry we wanted was seen.
var errFound = errors.New("found")

// Follow brings the index level with the journal.
//
// The index is rebuilt from entry 1 when it does not match the journal: when
// it is ahead, which happens if the journal lost a tail that was never
// confirmed, or when the entry it stopped at now has another seal, which
// happens if the journal was restored from a copy.
func (ix *Index) Follow(j *journal.Journal) error {
	if ix.last > 0 {
		matches := false
		err := j.Read(ix.last, func(e journal.Entry) error {
			matches = e.Seal() == ix.seal
			return errFound
		})
		if err != nil && !errors.Is(err, errFound) {
			return err
		}
		if !matches {
			if err := ix.reset(); err != nil {
				return err
			}
		}
	}

	const batch = 500
	pending := make([]journal.Entry, 0, batch)
	err := j.Read(ix.last+1, func(e journal.Entry) error {
		pending = append(pending, e)
		if len(pending) < batch {
			return nil
		}
		err := ix.Apply(pending)
		pending = pending[:0]
		return err
	})
	if err != nil {
		return err
	}
	return ix.Apply(pending)
}

// Last returns the number of the last entry applied.
func (ix *Index) Last() uint64 { return ix.last }

// DB gives read access to the index. Screens query it; only Apply writes.
func (ix *Index) DB() *sql.DB { return ix.db }

// Close closes the file.
func (ix *Index) Close() error { return ix.db.Close() }

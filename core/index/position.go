package index

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/earthler/earthler/core/journal"
)

// The meta table keeps the fingerprint of the schema, the format of the meta
// table itself, and one row per journal with the position of the index in
// it, such as "journal 0" = "17 <seal>". An index written in an older format
// is rebuilt.
const (
	metaSchema    = `CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID;`
	metaFormat    = "2"
	journalPrefix = "journal "
)

// load reads the positions of the index. It reports false when the file is
// new, was made with another schema or format, or cannot be read as an index.
func (ix *Index) load() (bool, error) {
	var schema, format string
	err := ix.db.QueryRow(`SELECT
		(SELECT value FROM meta WHERE key = 'schema'),
		(SELECT value FROM meta WHERE key = 'format')`).Scan(&schema, &format)
	if err != nil || schema != ix.fingerprint() || format != metaFormat {
		return false, nil // not an index we can use; it will be rebuilt
	}
	rows, err := ix.db.Query(`SELECT key, value FROM meta WHERE key LIKE 'journal %'`)
	if err != nil {
		return false, nil
	}
	defer rows.Close()
	at := map[uint64]Position{}
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) != nil {
			return false, nil
		}
		machine, err1 := strconv.ParseUint(strings.TrimPrefix(key, journalPrefix), 10, 64)
		pos, err2 := parsePosition(value)
		if err1 != nil || err2 != nil {
			return false, nil
		}
		at[machine] = pos
	}
	ix.setAt(at)
	return rows.Err() == nil, nil
}

func parsePosition(value string) (Position, error) {
	var p Position
	last, seal, ok := strings.Cut(value, " ")
	if !ok {
		return p, errors.New("no seal")
	}
	n, err := strconv.ParseUint(last, 10, 64)
	if err != nil {
		return p, err
	}
	raw, err := hex.DecodeString(seal)
	if err != nil || len(raw) != len(p.Seal) {
		return p, errors.New("not a seal")
	}
	p.Last = n
	copy(p.Seal[:], raw)
	return p, nil
}

func (ix *Index) saveSchema(tx *sql.Tx) error {
	for key, value := range map[string]string{"schema": ix.fingerprint(), "format": metaFormat} {
		if err := saveMeta(tx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func savePosition(tx *sql.Tx, machine uint64, p Position) error {
	return saveMeta(tx, journalPrefix+strconv.FormatUint(machine, 10), fmt.Sprintf("%d %s", p.Last, p.Seal))
}

func saveMeta(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// errFound ends a journal read once the entry we wanted was seen.
var errFound = errors.New("found")

// Follow brings the index level with every journal the machine holds, by
// machine number.
//
// The index is rebuilt from the start when it does not match them: when it
// is ahead of a journal, which happens if the journal lost a tail that was
// never confirmed; when the entry it stopped at now has another seal, which
// happens if a journal was restored from a copy; and when it holds entries of
// a journal that is no longer there.
func (ix *Index) Follow(journals map[uint64]*journal.Journal) error {
	matches := true
	ix.mu.Lock()
	at := make(map[uint64]Position, len(ix.at))
	for m, p := range ix.at {
		at[m] = p
	}
	ix.mu.Unlock()
	for machine, pos := range at {
		j := journals[machine]
		if j == nil {
			matches = false
			break
		}
		if pos.Last == 0 {
			continue
		}
		same := false
		err := j.Read(pos.Last, func(e journal.Entry) error {
			same = e.Seal() == pos.Seal
			return errFound
		})
		if err != nil && !errors.Is(err, errFound) {
			return err
		}
		if !same {
			matches = false
			break
		}
	}
	if !matches {
		if err := ix.reset(); err != nil {
			return err
		}
	}

	machines := make([]uint64, 0, len(journals))
	for m := range journals {
		machines = append(machines, m)
	}
	sort.Slice(machines, func(a, b int) bool { return machines[a] < machines[b] })
	for _, m := range machines {
		if err := ix.FollowOne(m, journals[m]); err != nil {
			return err
		}
	}
	return nil
}

// FollowOne applies what the index has not seen yet of one journal, such as
// the journal of a machine that has just joined. Unlike Follow, it never
// rebuilds the index.
func (ix *Index) FollowOne(machine uint64, j *journal.Journal) error {
	const batch = 500
	pending := make([]journal.Entry, 0, batch)
	err := j.Read(ix.Last(machine)+1, func(e journal.Entry) error {
		pending = append(pending, e)
		if len(pending) < batch {
			return nil
		}
		err := ix.Apply(machine, pending)
		pending = pending[:0]
		return err
	})
	if err != nil {
		return err
	}
	return ix.Apply(machine, pending)
}

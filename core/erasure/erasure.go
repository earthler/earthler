// Package erasure takes deleted content out of the journals for good. A
// deletion only hides a text: the entry that holds it stays in the journal
// until this job rewrites its segment with the content replaced by the
// erased mark. It runs when the system starts and once a night.
//
// Each machine erases its own files: its own journal and the copies it keeps
// of the other machines' journals. So what is waiting to be erased is a
// matter of each machine, and a record of an erasure only counts for the
// machine that wrote it.
package erasure

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/scribe"
)

// Area returns the index of what machine self still has to erase. It is
// derived from the journals like every index: a deletion adds the entry, and
// this machine's record of an erasure removes it, so a rebuilt index asks for
// the same work again, and erasing an entry twice changes nothing. erased
// remembers what this machine already erased, because another machine's
// deletion of the same message can arrive after the erasure.
func Area(self uint64) index.Area {
	return index.Area{
		Name: "erasure",
		// The machine is in the schema so the index is rebuilt if it were
		// ever opened by another one.
		Schema: fmt.Sprintf(`-- machine %d
			CREATE TABLE pending (entry INTEGER PRIMARY KEY, asked INTEGER NOT NULL);
			CREATE TABLE erased (entry INTEGER PRIMARY KEY);`, self),
		Apply: func(tx *sql.Tx, entry index.Entry) error { return apply(tx, entry, self) },
	}
}

func apply(tx *sql.Tx, entry index.Entry, self uint64) error {
	e, err := event.Decode(entry.Content)
	if err != nil {
		return nil
	}
	switch e.Type {
	case event.MessageDeleted:
		var d event.MessageDeletedData
		if e.Read(&d) != nil {
			return nil
		}
		// Two machines can delete the same message. Whichever deletion
		// arrives first, the one that asked is the one with the lower id,
		// and nothing is asked again once this machine erased it.
		_, err = tx.Exec(`INSERT INTO pending (entry, asked) SELECT ?1, ?2
			WHERE NOT EXISTS (SELECT 1 FROM erased WHERE entry = ?1)
			ON CONFLICT (entry) DO UPDATE SET asked = min(asked, excluded.asked)`, d.Message, entry.ID)
	case event.EntryErased:
		var d event.EntryErasedData
		if entry.Machine != self || e.Read(&d) != nil {
			return nil
		}
		for _, id := range d.Entries {
			if _, err = tx.Exec(`DELETE FROM pending WHERE entry = ?`, id); err != nil {
				return err
			}
			if _, err = tx.Exec(`INSERT OR IGNORE INTO erased (entry) VALUES (?)`, id); err != nil {
				return err
			}
		}
	}
	return err
}

// batch is how many entries are erased and recorded together.
var batch = 1000

// Run erases everything that is pending in the journals the machine holds,
// by machine, a batch at a time, and records in its own journal that it did.
// A run does not stop until nothing it can reach is pending, so what was
// deleted before the night's copy is never in the copy. An entry of a journal
// the machine does not hold yet stays pending. It returns how many entries
// it erased.
func Run(ctx context.Context, journals map[uint64]*journal.Journal, ix *index.Index, s *scribe.Scribe, now time.Time) (int, error) {
	total := 0
	for {
		n, err := runBatch(ctx, journals, ix, s, now)
		total += n
		if err != nil || n == 0 {
			return total, err
		}
	}
}

func runBatch(ctx context.Context, journals map[uint64]*journal.Journal, ix *index.Index, s *scribe.Scribe, now time.Time) (int, error) {
	ids, err := pending(ctx, ix, journals)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	byMachine := map[uint64][]uint64{}
	for _, id := range ids {
		m, n := event.Split(id)
		byMachine[m] = append(byMachine[m], n)
	}
	for m, numbers := range byMachine {
		if err := journals[m].Erase(numbers...); err != nil {
			return 0, err
		}
	}
	content, err := event.New(event.EntryErased, 0, 0, now, event.EntryErasedData{Entries: ids})
	if err != nil {
		return 0, err
	}
	if _, err = s.Write(ctx, content); err != nil {
		return 0, err
	}
	// The record just written takes the entries out of pending, unless the
	// index belongs to another machine than the scribe, which would erase
	// the same entries forever.
	var still int
	if err := ix.DB().QueryRowContext(ctx, `SELECT count(*) FROM pending WHERE entry = ?`, ids[0]).Scan(&still); err != nil {
		return 0, err
	}
	if still > 0 {
		return 0, fmt.Errorf("erasure: the index is not machine %d's, so its record of the erasure does not count", s.Self())
	}
	return len(ids), nil
}

// pending returns up to a batch of ids waiting to be erased in the journals
// the machine holds, among the entries that have arrived: a copy that is
// behind cannot erase what it does not have yet.
func pending(ctx context.Context, ix *index.Index, journals map[uint64]*journal.Journal) ([]uint64, error) {
	rows, err := ix.DB().QueryContext(ctx, `SELECT entry FROM pending ORDER BY entry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint64
	for rows.Next() && len(ids) < batch {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m, n := event.Split(id)
		if j := journals[m]; j != nil {
			if last, _ := j.Last(); n <= last {
				ids = append(ids, id)
			}
		}
	}
	return ids, rows.Err()
}

// NextNight says when the next run is due: the next time the clock reaches
// the given hour.
func NextNight(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// Package erasure takes deleted content out of the journal for good. A
// deletion only hides a text: the entry that holds it stays in the journal
// until this job rewrites its segment with the content replaced by the
// erased mark. It runs when the system starts and once a night.
package erasure

import (
	"context"
	"database/sql"
	"time"

	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/scribe"
)

// Area is the index of what is waiting to be erased. It is derived from the
// journal like every index: a deletion adds the entry, and the record of an
// erasure removes it, so a rebuilt index asks for the same work again, and
// erasing an entry twice changes nothing.
var Area = index.Area{
	Name:   "erasure",
	Schema: `CREATE TABLE pending (entry INTEGER PRIMARY KEY, asked INTEGER NOT NULL);`,
	Apply:  apply,
}

func apply(tx *sql.Tx, entry journal.Entry) error {
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
		_, err = tx.Exec(`INSERT OR IGNORE INTO pending (entry, asked) VALUES (?, ?)`, d.Message, entry.Number)
	case event.EntryErased:
		var d event.EntryErasedData
		if e.Read(&d) != nil {
			return nil
		}
		for _, n := range d.Entries {
			if _, err = tx.Exec(`DELETE FROM pending WHERE entry = ?`, n); err != nil {
				return err
			}
		}
	}
	return err
}

// batch is how many entries one run erases at most.
const batch = 1000

// Run erases what is pending and records in the journal that it did. It
// returns how many entries it erased.
func Run(ctx context.Context, j *journal.Journal, ix *index.Index, s *scribe.Scribe, now time.Time) (int, error) {
	rows, err := ix.DB().QueryContext(ctx, `SELECT entry FROM pending ORDER BY entry LIMIT ?`, batch)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var numbers []uint64
	for rows.Next() {
		var n uint64
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
		numbers = append(numbers, n)
	}
	if err := rows.Err(); err != nil || len(numbers) == 0 {
		return 0, err
	}
	if err := j.Erase(numbers...); err != nil {
		return 0, err
	}
	content, err := event.New(event.EntryErased, 0, 0, now, event.EntryErasedData{Entries: numbers})
	if err != nil {
		return 0, err
	}
	_, err = s.Write(ctx, content)
	return len(numbers), err
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

package index

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/earthler/earthler/core/journal"
)

// byID is like notes, but keeps each text under the id of its entry, the
// way the areas of the system do.
var byID = Area{
	Name:   "byid",
	Schema: `CREATE TABLE notes (number INTEGER PRIMARY KEY, text TEXT NOT NULL);`,
	Apply: func(tx *sql.Tx, e Entry) error {
		var data struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(e.Content, &data); err != nil {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO notes (number, text) VALUES (?, ?)`, e.ID, data.Text)
		return err
	},
}

func TestEachJournalHasItsOwnPosition(t *testing.T) {
	root := t.TempDir()
	mine := openJournal(t, filepath.Join(root, "journal"))
	theirs := openJournal(t, filepath.Join(root, "journals", "4"))
	write(t, mine, "one", "two", "three")
	write(t, theirs, "um", "dois")
	both := map[uint64]*journal.Journal{0: mine, 4: theirs}

	dir := filepath.Join(root, "indexes")
	ix := openIndex(t, dir, byID)
	if err := ix.Follow(both); err != nil {
		t.Fatal(err)
	}
	want := "1=one\n2=two\n3=three\n4000000000001=um\n4000000000002=dois\n"
	if ix.Last(0) != 3 || ix.Last(4) != 2 || dump(t, ix) != want {
		t.Fatalf("positions %d and %d, rows:\n%s", ix.Last(0), ix.Last(4), dump(t, ix))
	}

	// Reopened, the index knows where it was in each journal and only
	// applies what is new.
	ix.Close()
	write(t, theirs, "três")
	ix = openIndex(t, dir, byID)
	if ix.Last(0) != 3 || ix.Last(4) != 2 {
		t.Fatalf("reopened at %d and %d", ix.Last(0), ix.Last(4))
	}
	if err := ix.Follow(both); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, ix); got != want+"4000000000003=três\n" {
		t.Fatalf("rows after reopening:\n%s", got)
	}
}

func TestEntriesOfOneJournalComeInOrder(t *testing.T) {
	ix := openIndex(t, t.TempDir(), byID)
	one := journal.Entry{Number: 1, Content: []byte(`{"text":"one"}`)}
	two := journal.Entry{Number: 2, Content: []byte(`{"text":"two"}`)}
	if err := ix.Apply(4, []journal.Entry{two}); err == nil {
		t.Fatal("entry 2 of a journal was applied before entry 1")
	}
	// Another journal has its own count: entry 1 of machine 5 is fine even
	// though machine 4 is at entry 2.
	if err := ix.Apply(4, []journal.Entry{one, two}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Apply(5, []journal.Entry{one}); err != nil {
		t.Fatal(err)
	}
	if err := ix.Apply(9001, []journal.Entry{one}); err == nil {
		t.Fatal("a machine past the highest number was applied")
	}
}

func TestAJournalThatIsGoneRebuildsTheIndex(t *testing.T) {
	root := t.TempDir()
	mine := openJournal(t, filepath.Join(root, "journal"))
	theirs := openJournal(t, filepath.Join(root, "journals", "4"))
	write(t, mine, "one")
	write(t, theirs, "um")
	ix := openIndex(t, filepath.Join(root, "indexes"), byID)
	if err := ix.Follow(map[uint64]*journal.Journal{0: mine, 4: theirs}); err != nil {
		t.Fatal(err)
	}

	// Without the copy of machine 4, its entries cannot stay: the index
	// shows what the journals the machine holds say, and nothing else.
	if err := ix.Follow(only(mine)); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, ix); got != "1=one\n" || ix.Last(4) != 0 {
		t.Fatalf("rows after the copy went away, at %d:\n%s", ix.Last(4), got)
	}
}

func TestARestoredCopyRebuildsTheIndex(t *testing.T) {
	root := t.TempDir()
	mine := openJournal(t, filepath.Join(root, "journal"))
	first := openJournal(t, filepath.Join(root, "first"))
	second := openJournal(t, filepath.Join(root, "second"))
	write(t, mine, "one")
	write(t, first, "um", "dois")
	write(t, second, "uno", "dos", "tres")
	ix := openIndex(t, filepath.Join(root, "indexes"), byID)
	if err := ix.Follow(map[uint64]*journal.Journal{0: mine, 4: first}); err != nil {
		t.Fatal(err)
	}
	// Another history for machine 4: the seal at entry 2 is not the one the
	// index stopped at.
	if err := ix.Follow(map[uint64]*journal.Journal{0: mine, 4: second}); err != nil {
		t.Fatal(err)
	}
	want := "1=one\n4000000000001=uno\n4000000000002=dos\n4000000000003=tres\n"
	if got := dump(t, ix); got != want {
		t.Fatalf("rows after another history:\n%s", got)
	}
}

func TestAnIndexInTheOldFormatIsRebuilt(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, filepath.Join(root, "journal"))
	write(t, j, "one", "two")
	dir := filepath.Join(root, "indexes")
	ix := openIndex(t, dir, byID)
	if err := ix.Follow(only(j)); err != nil {
		t.Fatal(err)
	}
	// What an index written before many machines looked like: one position,
	// with no format.
	for _, q := range []string{`DELETE FROM meta WHERE key != 'schema'`, `INSERT INTO meta VALUES ('last', '2')`} {
		if _, err := ix.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ix.Close()
	ix = openIndex(t, dir, byID)
	if ix.Last(0) != 0 || dump(t, ix) != "" {
		t.Fatalf("an old index was kept at %d:\n%s", ix.Last(0), dump(t, ix))
	}
	if err := ix.Follow(only(j)); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, ix); got != "1=one\n2=two\n" {
		t.Fatalf("rows after the rebuild:\n%s", got)
	}
}

package index

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/earthler/earthler/core/journal"
)

// notes is a small area for the tests: one row for each entry that has text.
var notes = Area{
	Name:   "notes",
	Schema: `CREATE TABLE notes (number INTEGER PRIMARY KEY, text TEXT NOT NULL);`,
	Apply: func(tx *sql.Tx, e journal.Entry) error {
		var data struct {
			Text string `json:"text"`
			Fail bool   `json:"fail"`
		}
		if err := json.Unmarshal(e.Content, &data); err != nil {
			return err
		}
		if data.Fail {
			return errors.New("asked to fail")
		}
		_, err := tx.Exec(`INSERT INTO notes (number, text) VALUES (?, ?)`, e.Number, data.Text)
		return err
	},
}

func openJournal(t *testing.T, dir string) *journal.Journal {
	t.Helper()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	return j
}

func write(t *testing.T, j *journal.Journal, texts ...string) {
	t.Helper()
	var contents [][]byte
	for _, text := range texts {
		c, _ := json.Marshal(map[string]string{"text": text})
		contents = append(contents, c)
	}
	if _, err := j.Append(contents...); err != nil {
		t.Fatal(err)
	}
}

func openIndex(t *testing.T, dir string, area Area) *Index {
	t.Helper()
	ix, err := Open(dir, area)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

// dump returns the whole notes table as text, to compare two indexes.
func dump(t *testing.T, ix *Index) string {
	t.Helper()
	rows, err := ix.DB().Query(`SELECT number, text FROM notes ORDER BY number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var n uint64
		var text string
		if err := rows.Scan(&n, &text); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%d=%s\n", n, text)
	}
	return b.String()
}

func TestFollowAppliesTheJournal(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, filepath.Join(root, "journal"))
	write(t, j, "one", "two", "three")

	ix := openIndex(t, filepath.Join(root, "indexes"), notes)
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	if ix.Last() != 3 || dump(t, ix) != "1=one\n2=two\n3=three\n" {
		t.Fatalf("last = %d, rows:\n%s", ix.Last(), dump(t, ix))
	}

	write(t, j, "four")
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	ix.Close()

	ix = openIndex(t, filepath.Join(root, "indexes"), notes)
	if ix.Last() != 4 || dump(t, ix) != "1=one\n2=two\n3=three\n4=four\n" {
		t.Fatalf("after reopening: last = %d, rows:\n%s", ix.Last(), dump(t, ix))
	}
}

func TestDeletedIndexComesBackIdentical(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, filepath.Join(root, "journal"))
	for i := 0; i < 1200; i++ { // more than one batch of Follow
		write(t, j, fmt.Sprintf("note %d", i))
	}
	dir := filepath.Join(root, "indexes")
	ix := openIndex(t, dir, notes)
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	before := dump(t, ix)
	ix.Close()

	for _, name := range []string{"notes.db", "notes.db-wal", "notes.db-shm"} {
		os.Remove(filepath.Join(dir, name))
	}
	ix = openIndex(t, dir, notes)
	if ix.Last() != 0 {
		t.Fatalf("a new index starts at entry %d", ix.Last())
	}
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	if after := dump(t, ix); after != before || ix.Last() != 1200 {
		t.Fatal("the rebuilt index differs from the one that was deleted")
	}
}

func TestCorruptedIndexIsRebuilt(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, filepath.Join(root, "journal"))
	write(t, j, "one", "two")
	dir := filepath.Join(root, "indexes")
	ix := openIndex(t, dir, notes)
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	ix.Close()
	os.Remove(filepath.Join(dir, "notes.db-wal"))
	os.Remove(filepath.Join(dir, "notes.db-shm"))
	if err := os.WriteFile(filepath.Join(dir, "notes.db"), bytes.Repeat([]byte("garbage "), 2000), 0o600); err != nil {
		t.Fatal(err)
	}

	ix = openIndex(t, dir, notes)
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	if dump(t, ix) != "1=one\n2=two\n" {
		t.Fatalf("rows after rebuilding:\n%s", dump(t, ix))
	}
}

func TestIndexAheadOfTheJournalIsRebuilt(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "indexes")

	long := openJournal(t, filepath.Join(root, "long"))
	write(t, long, "one", "two", "three", "four", "five")
	ix := openIndex(t, dir, notes)
	if err := ix.Follow(long); err != nil {
		t.Fatal(err)
	}

	// The same history, but the journal lost its last two entries.
	short := openJournal(t, filepath.Join(root, "short"))
	write(t, short, "one", "two", "three")
	if err := ix.Follow(short); err != nil {
		t.Fatal(err)
	}
	if ix.Last() != 3 || dump(t, ix) != "1=one\n2=two\n3=three\n" {
		t.Fatalf("last = %d, rows:\n%s", ix.Last(), dump(t, ix))
	}
}

func TestIndexFromAnotherHistoryIsRebuilt(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "indexes")

	first := openJournal(t, filepath.Join(root, "first"))
	write(t, first, "one", "two")
	ix := openIndex(t, dir, notes)
	if err := ix.Follow(first); err != nil {
		t.Fatal(err)
	}

	// Same length, different second entry: as after restoring an older copy.
	second := openJournal(t, filepath.Join(root, "second"))
	write(t, second, "one", "another two", "three")
	if err := ix.Follow(second); err != nil {
		t.Fatal(err)
	}
	if dump(t, ix) != "1=one\n2=another two\n3=three\n" {
		t.Fatalf("rows:\n%s", dump(t, ix))
	}
}

func TestChangedSchemaRebuilds(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, filepath.Join(root, "journal"))
	write(t, j, "one", "two")
	dir := filepath.Join(root, "indexes")
	ix := openIndex(t, dir, notes)
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	ix.Close()

	changed := notes
	changed.Schema = `CREATE TABLE notes (number INTEGER PRIMARY KEY, text TEXT NOT NULL, extra TEXT);`
	ix = openIndex(t, dir, changed)
	if ix.Last() != 0 {
		t.Fatalf("an index with a new schema starts at entry %d", ix.Last())
	}
	if err := ix.Follow(j); err != nil {
		t.Fatal(err)
	}
	if dump(t, ix) != "1=one\n2=two\n" {
		t.Fatalf("rows:\n%s", dump(t, ix))
	}
}

func TestFailedBatchChangesNothing(t *testing.T) {
	root := t.TempDir()
	j := openJournal(t, filepath.Join(root, "journal"))
	write(t, j, "one")
	if _, err := j.Append([]byte(`{"text":"two"}`), []byte(`{"text":"three","fail":true}`)); err != nil {
		t.Fatal(err)
	}
	ix := openIndex(t, filepath.Join(root, "indexes"), notes)
	if err := ix.Follow(j); err == nil {
		t.Fatal("the failing entry was applied")
	}
	if ix.Last() != 0 || dump(t, ix) != "" {
		t.Fatalf("a failed batch left last = %d and rows:\n%s", ix.Last(), dump(t, ix))
	}
}

func TestEntriesMustBeInOrder(t *testing.T) {
	ix := openIndex(t, t.TempDir(), notes)
	err := ix.Apply([]journal.Entry{{Number: 2, Content: []byte(`{"text":"two"}`)}})
	if err == nil {
		t.Fatal("entry 2 was applied to an empty index")
	}
}

func TestErasedEntryIsSkipped(t *testing.T) {
	ix := openIndex(t, t.TempDir(), notes)
	entries := []journal.Entry{
		{Number: 1, Content: []byte(`{"text":"one"}`)},
		{Number: 2, Erased: true},
		{Number: 3, Content: []byte(`{"text":"three"}`)},
	}
	if err := ix.Apply(entries); err != nil {
		t.Fatal(err)
	}
	if ix.Last() != 3 || dump(t, ix) != "1=one\n3=three\n" {
		t.Fatalf("last = %d, rows:\n%s", ix.Last(), dump(t, ix))
	}
}

// TestFullTextSearchIsAvailable settles an open question of the design: the
// pure Go SQLite has to come with FTS5, which the community search needs.
func TestFullTextSearchIsAvailable(t *testing.T) {
	area := Area{
		Name:   "fts",
		Schema: `CREATE VIRTUAL TABLE docs USING fts5(body, tokenize = "unicode61 remove_diacritics 2");`,
		Apply: func(tx *sql.Tx, e journal.Entry) error {
			_, err := tx.Exec(`INSERT INTO docs (rowid, body) VALUES (?, ?)`, e.Number, string(e.Content))
			return err
		},
	}
	ix := openIndex(t, t.TempDir(), area)
	err := ix.Apply([]journal.Entry{
		{Number: 1, Content: []byte(`{"text":"a horta precisa de água"}`)},
		{Number: 2, Content: []byte(`{"text":"reunião amanhã"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var found uint64
	if err := ix.DB().QueryRow(`SELECT rowid FROM docs WHERE docs MATCH 'agua'`).Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatalf("searching without the accent found entry %d, want 1", found)
	}
	var version string
	if err := ix.DB().QueryRow(`SELECT sqlite_version()`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("SQLite %s with FTS5", version)
}

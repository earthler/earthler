package index

import (
	"database/sql"
	"testing"

	"github.com/earthler/earthler/core/journal"
)

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

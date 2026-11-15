package main

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/erasure"
	"github.com/earthler/earthler/core/event"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/screens/auditlog"
	"github.com/earthler/earthler/screens/chat"
	"github.com/earthler/earthler/screens/search"
)

// line is one entry to write in a test journal.
type line struct {
	typ             string
	account, device uint64
	at              int64
	data            any
}

func writeJournal(t *testing.T, dir string, lines []line) []journal.Entry {
	t.Helper()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var contents [][]byte
	for _, l := range lines {
		c, err := event.New(l.typ, l.account, l.device, time.UnixMilli(l.at), l.data)
		if err != nil {
			t.Fatal(err)
		}
		contents = append(contents, c)
	}
	entries, err := j.Append(contents...)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// twoMachines is a morning in a community with two machines. Machine 0
// founded it; Ana (3) and Bia (4) signed up there. Both machines took
// messages at the same time, and the two deleted the same message.
func twoMachines(t *testing.T) (zero, one []journal.Entry) {
	root := t.TempDir()
	id1 := func(n uint64) uint64 { return event.ID(1, n) }
	zero = writeJournal(t, filepath.Join(root, "0"), []line{
		{event.CommunityCreated, 0, 0, 900, event.CommunityCreatedData{Name: "Vila"}},
		{event.ChannelCreated, 0, 0, 900, event.ChannelCreatedData{Name: "Geral"}},
		{event.AccountCreated, 0, 0, 950, event.AccountCreatedData{Name: "Ana", FirstDevice: true}},
		{event.AccountCreated, 0, 0, 960, event.AccountCreatedData{Name: "Bia", FirstDevice: true}},
		{event.MessageCreated, 3, 3, 1000, event.MessageCreatedData{Channel: 2, Text: "bom dia"}},
		{event.MessageCreated, 4, 4, 1010, event.MessageCreatedData{Channel: 2, Text: "a horta precisa de água"}},
		{event.MessageDeleted, 3, 3, 1030, event.MessageDeletedData{Channel: 2, Message: 5, Placed: 1000}},
		{event.RoleChanged, 3, 3, 1040, event.RoleChangedData{Account: 4, Role: "moderator"}},
		// Ana removes Bia's message here, while on machine 1 Bia had
		// already deleted it herself: the earlier deletion stays.
		{event.MessageDeleted, 3, 3, 1032, event.MessageDeletedData{Channel: 2, Message: 6, Author: 4, Reason: "repetida", Placed: 1010}},
		// Machine 0 erased both messages in its journal, before machine
		// 1's deletion of message 6 reached it, in some of the orders.
		{event.EntryErased, 0, 0, 1100, event.EntryErasedData{Entries: []uint64{5, 6}}},
	})
	one = writeJournal(t, filepath.Join(root, "1"), []line{
		{event.MessageCreated, 4, 4, 1005, event.MessageCreatedData{Channel: 2, Text: "reunião amanhã"}},
		{event.MessageDeleted, 4, 4, 1031, event.MessageDeletedData{Channel: 2, Message: 6, Placed: 1010}},
		{event.MessageCreated, 3, 3, 1050, event.MessageCreatedData{Channel: 2, Text: "olá da outra casa"}},
		{event.MessageDeleted, 4, 4, 1060, event.MessageDeletedData{Channel: 2, Message: id1(1), Placed: 1005}},
		{event.EntryErased, 0, 0, 1100, event.EntryErasedData{Entries: []uint64{6}}},
	})
	return zero, one
}

// manyAreas are the areas whose tables must come out the same whatever the
// order the two journals arrive in.
func manyAreas() []index.Area {
	return []index.Area{accounts.Area, chat.Area, search.Area, auditlog.Area, erasure.Area(0)}
}

// applyInterleaved applies the two journals to fresh indexes, each journal
// in its own order, mixing them by the given order, and returns what every
// table holds.
func applyInterleaved(t *testing.T, zero, one []journal.Entry, order []uint64) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	for _, area := range manyAreas() {
		ix, err := index.Open(dir, area)
		if err != nil {
			t.Fatal(err)
		}
		next := map[uint64]int{}
		for _, m := range order {
			from := map[uint64][]journal.Entry{0: zero, 1: one}[m]
			if err := ix.Apply(m, from[next[m]:next[m]+1]); err != nil {
				t.Fatal(err)
			}
			next[m]++
		}
		fmt.Fprintf(&b, "== %s\n%s", area.Name, tables(t, ix))
		ix.Close()
	}
	return b.String()
}

// tables writes out every table of an index, except the meta table and the
// inner tables of the full text search, whose layout depends on the order
// of insertion; the search terms are read through their table instead.
func tables(t *testing.T, ix *index.Index) string {
	t.Helper()
	rows, err := ix.DB().Query(`SELECT name FROM sqlite_master WHERE type = 'table'
		AND name != 'meta' AND name NOT LIKE 'terms_%' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	var b strings.Builder
	for _, name := range names {
		q := `SELECT * FROM ` + name + ` ORDER BY 1`
		if name == "terms" {
			q = `SELECT rowid, body FROM terms ORDER BY rowid`
		}
		rows, err := ix.DB().Query(q)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			rows.Scan(ptrs...)
			fmt.Fprintf(&b, "%s %v\n", name, values)
		}
		rows.Close()
	}
	return b.String()
}

func TestTwoJournalsMakeTheSameIndexesInAnyOrder(t *testing.T) {
	zero, one := twoMachines(t)
	var orders [][]uint64
	all := func(m uint64, n int) []uint64 {
		var o []uint64
		for range n {
			o = append(o, m)
		}
		return o
	}
	orders = append(orders, append(all(0, len(zero)), all(1, len(one))...))
	orders = append(orders, append(all(1, len(one)), all(0, len(zero))...))
	random := rand.New(rand.NewSource(1))
	for range 20 {
		o := append(all(0, len(zero)), all(1, len(one))...)
		random.Shuffle(len(o), func(a, b int) { o[a], o[b] = o[b], o[a] })
		orders = append(orders, o)
	}
	want := applyInterleaved(t, zero, one, orders[0])
	for i, o := range orders[1:] {
		if got := applyInterleaved(t, zero, one, o); got != want {
			t.Fatalf("order %d (%v) made other tables.\nfirst:\n%s\nthis one:\n%s", i+1, o, want, got)
		}
	}

	// And the tables say what happened that morning.
	for _, must := range []string{
		// The conversation in the order of time, across the two machines:
		// number, channel, account, place, time, text, deleted.
		"messages [5 2 3 1000 1030  7]",
		"messages [1000000000001 2 4 1005 1060  1000000000004]",
		"messages [6 2 4 1010 1031  1000000000002]",
		"messages [1000000000003 2 3 1050 1050 olá da outra casa 0]",
		// Search only finds what was not deleted.
		"docs [1000000000003 chat 3 1050 olá da outra casa]",
		// Ana's removal is an act in the log even though Bia's own
		// deletion came first.
		"acts [9 1032 3 removal 4 repetida]",
		"acts [8 1040 3 role 4 moderator]",
		// Machine 0 erased messages 5 and 6 in its journal, and still has
		// to erase machine 1's message in its copy: machine 1's own record
		// speaks only for machine 1.
		"pending [1000000000001 1000000000004]",
		"erased [5]",
		"erased [6]",
	} {
		if !strings.Contains(want, must+"\n") {
			t.Errorf("the tables do not have %q:\n%s", must, want)
		}
	}
	for _, gone := range []string{"pending [5 ", "pending [6 "} {
		if strings.Contains(want, gone) {
			t.Errorf("%q: a message machine 0 erased is still pending:\n%s", gone, want)
		}
	}
}

// A deletion written before deletions carried the time of the message puts
// the message in the same place whether its text is still in the journal or
// was erased and the index rebuilt.
func TestAnOldDeletionLeavesTheSameRowAfterErasure(t *testing.T) {
	entries := writeJournal(t, t.TempDir(), []line{
		{event.CommunityCreated, 0, 0, 900, event.CommunityCreatedData{Name: "Vila"}},
		{event.ChannelCreated, 0, 0, 900, event.ChannelCreatedData{Name: "Geral"}},
		{event.AccountCreated, 0, 0, 950, event.AccountCreatedData{Name: "Ana", FirstDevice: true}},
		{event.MessageCreated, 3, 3, 1000, event.MessageCreatedData{Channel: 2, Text: "apaga"}},
		{event.MessageCreated, 3, 3, 1010, event.MessageCreatedData{Channel: 2, Text: "fica"}},
		{event.MessageDeleted, 3, 3, 2000, event.MessageDeletedData{Channel: 2, Message: 4}},
	})
	erased := append([]journal.Entry(nil), entries...)
	erased[3].Content, erased[3].Erased = nil, true
	dump := func(es []journal.Entry) string {
		ix, err := index.Open(t.TempDir(), chat.Area)
		if err != nil {
			t.Fatal(err)
		}
		defer ix.Close()
		if err := ix.Apply(0, es); err != nil {
			t.Fatal(err)
		}
		return tables(t, ix)
	}
	if before, after := dump(entries), dump(erased); before != after {
		t.Fatalf("before the erasure:\n%s\nafter:\n%s", before, after)
	}
}

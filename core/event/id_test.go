package event

import (
	"bytes"
	"testing"
	"time"
)

func TestAnIDReadsAsMachineAndEntry(t *testing.T) {
	if got := ID(3, 17); got != 3000000000017 {
		t.Fatalf("ID(3, 17) = %d", got)
	}
	if m, n := Split(3000000000017); m != 3 || n != 17 {
		t.Fatalf("Split = %d, %d", m, n)
	}
	// The founding machine's ids are its entry numbers, so the journals
	// written before there were many machines need nothing new.
	if ID(0, 42) != 42 {
		t.Fatal("the founding machine's ids must be its entry numbers")
	}
	if got := ID(MaxMachine, MachineSpan-1); got >= 1<<53 {
		t.Fatalf("the largest id, %d, is past what a browser script reads exactly", got)
	}
	for _, c := range []struct {
		machine, entry uint64
		valid          bool
	}{
		{0, 1, true}, {0, 0, false}, {MaxMachine, 1, true}, {MaxMachine + 1, 1, false},
		{1, MachineSpan - 1, true}, {1, MachineSpan, false},
	} {
		if Valid(c.machine, c.entry) != c.valid {
			t.Errorf("Valid(%d, %d) = %v", c.machine, c.entry, !c.valid)
		}
	}
}

func TestStampNeverGoesBack(t *testing.T) {
	at := time.UnixMilli(5000)
	content, err := New(MessageCreated, 7, 8, at, MessageCreatedData{Channel: 2, Text: "olá"})
	if err != nil {
		t.Fatal(err)
	}

	// A floor at or before the time changes nothing, byte for byte.
	for _, floor := range []int64{0, 4999, 5000} {
		got, when, err := Stamp(content, floor)
		if err != nil || when != 5000 || !bytes.Equal(got, content) {
			t.Fatalf("floor %d: time %d, changed %v, err %v", floor, when, !bytes.Equal(got, content), err)
		}
	}

	// A later floor moves the time and keeps everything else.
	got, when, err := Stamp(content, 9000)
	if err != nil || when != 9000 {
		t.Fatalf("time %d, err %v", when, err)
	}
	before, _ := Decode(content)
	after, err := Decode(got)
	if err != nil || after.Time != 9000 || after.Type != before.Type || after.Account != 7 || after.Device != 8 ||
		after.Salt != before.Salt || !bytes.Equal(after.Data, before.Data) {
		t.Fatalf("stamped event changed more than its time:\n%s\n%s", content, got)
	}

	// What is not an event is left alone.
	plain := []byte(`{"text":"not an event"}`)
	if got, when, err := Stamp(plain, 9000); err != nil || when != 0 || !bytes.Equal(got, plain) {
		t.Fatalf("a content that is not an event was changed: %s, %d, %v", got, when, err)
	}
}

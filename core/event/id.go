package event

import (
	"encoding/json"
	"fmt"
)

// Every thing in the community is known by the machine whose journal holds
// the entry that created it, and by the number of that entry. Both fit in
// one number, machine × MachineSpan + entry, so an id written in decimal can
// be read with the eye: 3000000000017 is entry 17 of machine 3.
//
// The machine that founded the community is machine 0, so its ids are its
// entry numbers, and a journal written before there were many machines is
// already in this form.
const (
	// MachineSpan is how many entries one journal can number.
	MachineSpan = 1_000_000_000_000

	// MaxMachine is the highest machine number. It keeps every id below
	// 2^53, where a script in the browser still reads a number exactly.
	MaxMachine = 9000
)

// ID returns the id of entry number entry of machine's journal. Valid says
// whether the two are in range; ID does not check.
func ID(machine, entry uint64) uint64 { return machine*MachineSpan + entry }

// Valid reports whether entry number entry of machine's journal has an id.
func Valid(machine, entry uint64) bool {
	return machine <= MaxMachine && entry > 0 && entry < MachineSpan
}

// Split returns the machine and the entry number of an id.
func Split(id uint64) (machine, entry uint64) { return id / MachineSpan, id % MachineSpan }

// TimeOf returns the time written in a content, or zero when the content is
// not an event.
func TimeOf(content []byte) int64 {
	var e struct {
		Type string `json:"type"`
		Time int64  `json:"time"`
	}
	if json.Unmarshal(content, &e) != nil || e.Type == "" {
		return 0
	}
	return e.Time
}

// Stamp makes sure that an event about to be written carries a time no
// earlier than floor. Across the journals of different machines the order is
// the time, and the scribe calls Stamp so that no entry gets a time earlier
// than one the machine has already seen: an answer then always comes after
// its question, even when a clock is wrong.
//
// It returns the content, changed only when its time was earlier than floor,
// and the time it carries. A content that is not an event is returned as it
// is, with a time of zero.
func Stamp(content []byte, floor int64) ([]byte, int64, error) {
	t := TimeOf(content)
	if t == 0 || t >= floor {
		return content, t, nil
	}
	e, err := Decode(content)
	if err != nil {
		return nil, 0, err
	}
	e.Time = floor
	out, err := json.Marshal(e)
	if err != nil {
		return nil, 0, fmt.Errorf("event: %w", err)
	}
	return out, floor, nil
}

package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/earthler/earthler/core/journal"
)

var errFound = errors.New("found")

// check opens the journal the way the system does after a power cut and
// compares it with what the collector heard. It tells the collector the
// verdict, so the tally of many power cuts builds up on the machine that
// stays on.
func check(dataDir, addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("the collector at %s does not answer: %w", addr, err)
	}
	defer conn.Close()
	answers := bufio.NewReader(conn)
	fmt.Fprintln(conn, "LAST")
	var confirmed uint64
	var seal string
	line, err := answers.ReadString('\n')
	if err == nil {
		_, err = fmt.Sscan(line, &confirmed, &seal)
	}
	if err != nil {
		return fmt.Errorf("the collector gave no answer: %w", err)
	}

	ok, detail := inspect(dataDir, confirmed, seal)
	verdict := "FAIL"
	if ok {
		verdict = "PASS"
	}
	fmt.Printf("%s: %s\n", verdict, detail)
	fmt.Fprintf(conn, "RESULT %s %s\n", verdict, detail)
	if _, err := answers.ReadString('\n'); err != nil {
		return fmt.Errorf("the collector did not confirm that it recorded the verdict: %w", err)
	}
	if !ok {
		return errors.New("the journal lost something that was confirmed")
	}
	return nil
}

// inspect reports whether the journal in dir holds everything up to the
// confirmed entry, with the seal the writer reported for it.
func inspect(dir string, confirmed uint64, seal string) (bool, string) {
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		return false, "the journal did not open: " + err.Error()
	}
	defer j.Close()
	if err := j.Verify(); err != nil {
		return false, "the full check failed: " + err.Error()
	}
	last, _ := j.Last()
	if last < confirmed {
		return false, fmt.Sprintf("entry %d was confirmed, but the journal ends at entry %d", confirmed, last)
	}
	if confirmed > 0 {
		var got string
		err := j.Read(confirmed, func(e journal.Entry) error {
			got = e.Seal().String()
			return errFound
		})
		if err != nil && !errors.Is(err, errFound) {
			return false, "reading the confirmed entry failed: " + err.Error()
		}
		if got != seal {
			return false, fmt.Sprintf("entry %d is there, but with another seal", confirmed)
		}
	}
	return true, fmt.Sprintf("the journal ends at entry %d, %d were confirmed, %d were written and never confirmed",
		last, confirmed, last-confirmed)
}

// write checks the journal first, then writes without stopping and tells the
// collector about each confirmed entry. It only ends when the power is cut
// or the collector goes away.
func write(dataDir, addr string) error {
	if err := check(dataDir, addr); err != nil {
		return err
	}
	j, err := journal.Open(dataDir, journal.Options{})
	if err != nil {
		return err
	}
	defer j.Close()
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	fmt.Println("Writing. Pull the plug whenever you want.")
	return writeLoop(j, conn, nil)
}

// writeLoop appends batches of different sizes, so a power cut can land in
// the middle of a small write or of a large one. It reports an entry only
// after the journal confirmed it.
func writeLoop(j *journal.Journal, conn net.Conn, stop <-chan struct{}) error {
	shown := time.Now()
	for i := 1; ; i++ {
		select {
		case <-stop:
			return nil
		default:
		}
		batch := make([][]byte, 1+i%20)
		for k := range batch {
			batch[k] = []byte(fmt.Sprintf(`{"call":%d,"item":%d,"fill":"%s"}`, i, k, strings.Repeat("x", (i*53)%800)))
		}
		entries, err := j.Append(batch...)
		if err != nil {
			return err
		}
		last := entries[len(entries)-1]
		if _, err := fmt.Fprintf(conn, "CONFIRMED %d %s\n", last.Number, last.Seal()); err != nil {
			return fmt.Errorf("lost the collector, so there is no point in going on: %w", err)
		}
		if time.Since(shown) > time.Second {
			fmt.Printf("\rconfirmed up to entry %d", last.Number)
			shown = time.Now()
		}
	}
}

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/earthler/earthler/core/journal"
)

// startCollector runs a collector on a free local port.
func startCollector(t *testing.T, path string) (*collector, string) {
	t.Helper()
	c := &collector{path: path, seal: strings.Repeat("0", 64)}
	c.load()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go c.serve(ln)
	return c, ln.Addr().String()
}

// writeFor runs the writer for a moment and returns the last entry the
// collector heard about.
func writeFor(t *testing.T, dir, addr string, c *collector) uint64 {
	t.Helper()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- writeLoop(j, conn, stop) }()
	time.Sleep(150 * time.Millisecond)
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	last, _ := j.Last()
	j.Close()
	conn.Close()

	// The collector reads from the network at its own pace.
	deadline := time.Now().Add(5 * time.Second)
	for {
		c.mu.Lock()
		heard := c.number
		c.mu.Unlock()
		if heard == last {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("the collector heard up to entry %d, the writer confirmed %d", heard, last)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProofPassesWhenNothingIsLost(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "journal")
	c, addr := startCollector(t, filepath.Join(root, "collector.log"))

	if err := check(dir, addr); err != nil {
		t.Fatalf("an empty journal against an empty collector: %v", err)
	}
	last := writeFor(t, dir, addr, c)
	if last == 0 {
		t.Fatal("the writer confirmed nothing")
	}
	if err := check(dir, addr); err != nil {
		t.Fatalf("after writing %d entries: %v", last, err)
	}
	more := writeFor(t, dir, addr, c)
	if more <= last {
		t.Fatalf("the second run did not continue: %d then %d", last, more)
	}
	if err := check(dir, addr); err != nil {
		t.Fatal(err)
	}

	log, _ := os.ReadFile(filepath.Join(root, "collector.log"))
	if n := strings.Count(string(log), "RESULT PASS"); n != 3 {
		t.Fatalf("the collector recorded %d passes, want 3:\n%s", n, log)
	}

	// A collector that is restarted remembers where it was.
	again := &collector{path: filepath.Join(root, "collector.log")}
	again.load()
	if again.number != more {
		t.Fatalf("a restarted collector remembers entry %d, want %d", again.number, more)
	}
}

func TestProofFailsWhenAConfirmedEntryIsGone(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "journal")
	c, addr := startCollector(t, filepath.Join(root, "collector.log"))
	writeFor(t, dir, addr, c)

	// Cut the last line off the file: a confirmed entry that the disk lost.
	path := filepath.Join(dir, "00000000000000000001.journal")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.LastIndexByte(strings.TrimSuffix(string(data), "\n"), '\n') + 1
	if err := os.WriteFile(path, data[:cut], 0o600); err != nil {
		t.Fatal(err)
	}

	if err := check(dir, addr); err == nil {
		t.Fatal("the proof passed with a confirmed entry missing")
	}
	log, _ := os.ReadFile(filepath.Join(root, "collector.log"))
	if !strings.Contains(string(log), "RESULT FAIL") {
		t.Fatalf("the collector did not record the failure:\n%s", log)
	}
}

func TestProofFailsOnAnotherHistory(t *testing.T) {
	root := t.TempDir()
	c, addr := startCollector(t, filepath.Join(root, "collector.log"))
	writeFor(t, filepath.Join(root, "journal"), addr, c)

	// A journal with as many entries, but not the ones that were confirmed.
	other := filepath.Join(root, "other")
	j, err := journal.Open(other, journal.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < c.number; i += 50 {
		batch := make([][]byte, 50)
		for k := range batch {
			batch[k] = []byte(fmt.Sprintf(`{"other":%d}`, i+uint64(k)))
		}
		if _, err := j.Append(batch...); err != nil {
			t.Fatal(err)
		}
	}
	j.Close()
	if ok, detail := inspect(other, c.number, c.seal); ok {
		t.Fatalf("another history passed: %s", detail)
	}
}

func TestCollectorAnswersLast(t *testing.T) {
	c, addr := startCollector(t, filepath.Join(t.TempDir(), "collector.log"))
	c.confirm(42, strings.Repeat("ab", 32))
	c.confirm(7, strings.Repeat("cd", 32)) // an older report never moves it back
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintln(conn, "LAST")
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || line != "42 "+strings.Repeat("ab", 32)+"\n" {
		t.Fatalf("answer = %q, err = %v", line, err)
	}
}

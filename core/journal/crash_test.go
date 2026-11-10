package journal

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const crashWriterEnv = "JOURNAL_CRASH_WRITER"

// TestMain lets the test program run again as a writer that is going to be
// killed. See TestKilledWriterLosesNothingConfirmed.
func TestMain(m *testing.M) {
	if dir := os.Getenv(crashWriterEnv); dir != "" {
		crashWriter(dir)
		return
	}
	os.Exit(m.Run())
}

// crashWriter appends without stopping and prints the number of the last
// entry of each confirmed call. It uses the real confirmation.
func crashWriter(dir string) {
	j, err := Open(dir, Options{SegmentSize: 16 << 10})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	for i := 1; ; i++ {
		batch := make([][]byte, 1+i%5)
		for k := range batch {
			batch[k] = []byte(fmt.Sprintf(`{"call":%d,"item":%d,"fill":"%s"}`, i, k, strings.Repeat("x", (i*37)%900)))
		}
		added, err := j.Append(batch...)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		fmt.Println(added[len(added)-1].Number)
	}
}

// TestKilledWriterLosesNothingConfirmed kills a writer at a random moment,
// many times over the same journal. After each kill the journal must open,
// hold every entry the writer had confirmed, and pass the full check.
//
// Killing a program is not the same as cutting the power: the operating
// system still writes what the program handed over. This test covers a write
// interrupted halfway. The power cut proof needs a real machine.
func TestKilledWriterLosesNothingConfirmed(t *testing.T) {
	if testing.Short() {
		t.Skip("kills a writer many times; skipped with -short")
	}
	rounds := 25
	if v, err := strconv.Atoi(os.Getenv("JOURNAL_CRASH_ROUNDS")); err == nil && v > 0 {
		rounds = v
	}
	dir := t.TempDir()
	rng := rand.New(rand.NewSource(1))
	var confirmed uint64

	for round := 1; round <= rounds; round++ {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), crashWriterEnv+"="+dir)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		numbers := make(chan uint64)
		go func() {
			defer close(numbers)
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				if n, err := strconv.ParseUint(sc.Text(), 10, 64); err == nil {
					numbers <- n
				}
			}
		}()

		kill := time.After(time.Duration(30+rng.Intn(120)) * time.Millisecond)
		killed := false
		for numbers != nil {
			select {
			case n, ok := <-numbers:
				if !ok {
					numbers = nil
					break
				}
				confirmed = max(confirmed, n)
			case <-kill:
				if !killed {
					cmd.Process.Kill()
					killed = true
				}
			}
		}
		cmd.Wait()
		if !killed {
			t.Fatalf("round %d: the writer stopped by itself", round)
		}

		j, err := Open(dir, Options{SegmentSize: 16 << 10})
		if err != nil {
			t.Fatalf("round %d: open after the kill: %v", round, err)
		}
		last, _ := j.Last()
		if last < confirmed {
			t.Fatalf("round %d: entry %d was confirmed, but the journal ends at %d", round, confirmed, last)
		}
		if err := j.Verify(); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		j.Close()
	}
	if confirmed == 0 {
		t.Fatal("no entry was ever confirmed, so the test proved nothing")
	}
	t.Logf("%d kills, %d entries confirmed, none lost", rounds, confirmed)
}

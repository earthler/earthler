package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The two machines talk in lines of text over one TCP connection:
//
//	CONFIRMED <number> <seal>   the writer, after the disk confirmed an entry
//	LAST                        the checker asks; the answer is "<number> <seal>"
//	RESULT <text>               the checker, with the verdict of one power cut;
//	                            the answer is "OK" once it was written down

// collector remembers the highest entry the writer confirmed. It runs on the
// machine that stays on, so memory is enough while it runs. The file lets it
// be restarted between power cuts.
type collector struct {
	mu     sync.Mutex
	number uint64
	seal   string
	dirty  bool
	path   string
}

func collect(listen, path string) error {
	c := &collector{path: path, seal: strings.Repeat("0", 64)}
	c.load()
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	fmt.Printf("Collecting on %s. Highest confirmed entry so far: %d\n", ln.Addr(), c.number)
	go func() {
		for range time.Tick(time.Second) {
			c.save()
		}
	}()
	return c.serve(ln)
}

func (c *collector) serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go c.handle(conn)
	}
}

func (c *collector) handle(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		switch {
		case len(fields) == 3 && fields[0] == "CONFIRMED":
			if n, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				c.confirm(n, fields[2])
			}
		case len(fields) == 1 && fields[0] == "LAST":
			c.mu.Lock()
			fmt.Fprintf(conn, "%d %s\n", c.number, c.seal)
			c.mu.Unlock()
		case len(fields) > 1 && fields[0] == "RESULT":
			c.result(strings.Join(fields[1:], " "))
			fmt.Fprintln(conn, "OK")
		}
	}
}

func (c *collector) confirm(number uint64, seal string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if number > c.number {
		c.number, c.seal, c.dirty = number, seal, true
	}
}

// result writes the verdict of one power cut where a person can count them.
func (c *collector) result(text string) {
	c.save()
	line := time.Now().Format("2006-01-02 15:04:05") + " RESULT " + text
	fmt.Println(line)
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "journal-proof:", err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

// save appends the current position to the file, when it changed.
func (c *collector) save() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return
	}
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "journal-proof:", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "CONFIRMED %d %s\n", c.number, c.seal)
	c.dirty = false
}

// load reads the highest position from a file written by save.
func (c *collector) load() {
	f, err := os.Open(c.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 3 && fields[0] == "CONFIRMED" {
			if n, err := strconv.ParseUint(fields[1], 10, 64); err == nil && n > c.number {
				c.number, c.seal = n, fields[2]
			}
		}
	}
}

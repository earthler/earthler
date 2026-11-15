package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// secureServer is the secure site of one machine, on an address that stays
// the same when the machine goes away and comes back.
type secureServer struct {
	t       *testing.T
	addr    string
	cert    tls.Certificate
	handler func() http.Handler

	mu  sync.Mutex
	srv *http.Server
}

func newSecure(t *testing.T, cert tls.Certificate) *secureServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &secureServer{t: t, addr: ln.Addr().String(), cert: cert}
	s.serve(ln)
	t.Cleanup(s.stop)
	return s
}

func (s *secureServer) url() string { return "https://" + s.addr }

func (s *secureServer) serve(ln net.Listener) {
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := s.handler; h != nil && h() != nil {
			h().ServeHTTP(w, r)
			return
		}
		http.Error(w, "not yet", http.StatusServiceUnavailable)
	})}
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{s.cert}})
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()
	go srv.Serve(tlsLn)
}

// stop takes the machine off the network, as a power cut or a broken
// antenna would, with the program still running.
func (s *secureServer) stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
}

func (s *secureServer) start() {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.t.Fatal(err)
	}
	s.serve(ln)
}

// between returns the text between two marks.
func between(body, start, end string) string {
	_, rest, ok := strings.Cut(body, start)
	if !ok {
		return ""
	}
	text, _, _ := strings.Cut(rest, end)
	return text
}

// eventually waits for a condition that depends on another machine.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// order returns the texts of the conversation in the order a page shows them.
func order(body string, texts ...string) []string {
	type at struct {
		pos  int
		text string
	}
	var found []at
	for _, text := range texts {
		if i := strings.Index(body, text); i >= 0 {
			found = append(found, at{i, text})
		}
	}
	for i := range found {
		for j := i + 1; j < len(found); j++ {
			if found[j].pos < found[i].pos {
				found[i], found[j] = found[j], found[i]
			}
		}
	}
	var out []string
	for _, f := range found {
		out = append(out, f.text)
	}
	return out
}

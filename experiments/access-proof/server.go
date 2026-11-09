package main

import (
	"context"
	"crypto/tls"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed pages static
var embedded embed.FS

// server answers the welcome page over HTTP and the test page over HTTPS.
// Any name outside the community domain gets a forged certificate.
type server struct {
	chain   *chain
	cert    *tls.Certificate
	ips     []net.IP
	opts    options
	mdns    string // this machine's .local name, or empty
	pages   *template.Template
	static  fs.FS
	results *resultLog

	mu     sync.Mutex
	forged map[string]*tls.Certificate
	noted  map[string]bool
	asked  sync.Map // connection -> name the client asked for
}

func newServer(c *chain, cert *tls.Certificate, ips []net.IP, o options) (*server, error) {
	pages, err := template.ParseFS(embedded, "pages/*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, err
	}
	return &server{
		chain:   c,
		cert:    cert,
		ips:     ips,
		opts:    o,
		mdns:    mdnsName(),
		pages:   pages,
		static:  static,
		results: &resultLog{path: filepath.Join(o.dataDir, resultsFile)},
		forged:  map[string]*tls.Certificate{},
		noted:   map[string]bool{},
	}, nil
}

// isForged reports whether a name is outside what the community may sign.
// An empty name means the client connected by address.
func (s *server) isForged(name string) bool {
	if name == "" || net.ParseIP(name) != nil {
		return false
	}
	return name != s.chain.domain && !strings.HasSuffix(name, "."+s.chain.domain)
}

// certificateFor picks the certificate for a connection and remembers the
// name the client asked for, so the handshake result can be logged with it.
func (s *server) certificateFor(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(hello.ServerName)
	s.asked.Store(hello.Conn, name)
	if !s.isForged(name) {
		return s.cert, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cert, ok := s.forged[name]; ok {
		return cert, nil
	}
	cert, err := s.chain.issue(time.Now(), []string{name}, nil)
	if err != nil {
		return nil, err
	}
	s.forged[name] = cert
	return cert, nil
}

// acceptTLS finishes each handshake itself instead of leaving it to
// net/http. A finished handshake means the client accepted the certificate,
// and a failed one carries the reason, which is the evidence this experiment
// is after.
func (s *server) acceptTLS(ln net.Listener, ready *readyConns) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: s.certificateFor}
	for {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			conn := tls.Server(raw, cfg)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err := conn.HandshakeContext(ctx)
			asked, _ := s.asked.LoadAndDelete(raw)
			name, _ := asked.(string)
			s.noteHandshake(remoteIP(raw.RemoteAddr().String()), name, err)
			if err != nil {
				raw.Close()
				return
			}
			ready.deliver(conn)
		}()
	}
}

func (s *server) noteHandshake(ip, name string, err error) {
	if s.isForged(name) {
		res := result{Remote: ip, Scheme: "https", Host: name, Checks: map[string]string{}}
		res.Checks["device_trusts_real_certificate"] = boolText(s.wasNoted(ip + " trusted"))
		if err != nil {
			res.Scenario = "forged-rejected"
			res.Notes = err.Error()
			log.Printf("FORGED TEST  %s refused the forged certificate for %s. That is the right outcome. (%v)", ip, name, err)
		} else {
			res.Scenario = "forged-accepted"
			log.Printf("FORGED TEST  %s ACCEPTED the forged certificate for %s. The device does not enforce the limits.", ip, name)
		}
		if err := s.results.add(res); err != nil {
			log.Printf("save result: %v", err)
		}
		return
	}
	if err == nil {
		if s.firstTime(ip + " trusted") {
			log.Printf("%s trusts the community certificate.", ip)
		}
		return
	}
	if s.firstTime(ip + " " + err.Error()) {
		log.Printf("%s does not accept the community certificate yet. (%v)", ip, err)
	}
}

// firstTime reports whether key is new, and remembers it.
func (s *server) firstTime(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.noted[key] {
		return false
	}
	s.noted[key] = true
	return true
}

func (s *server) wasNoted(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.noted[key]
}

// readyConns is a net.Listener that hands out connections whose TLS
// handshake has already finished.
type readyConns struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	addr  net.Addr
}

func newReadyConns(addr net.Addr) *readyConns {
	return &readyConns{conns: make(chan net.Conn), done: make(chan struct{}), addr: addr}
}

func (l *readyConns) deliver(c net.Conn) {
	select {
	case l.conns <- c:
	case <-l.done:
		c.Close()
	}
}

func (l *readyConns) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *readyConns) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *readyConns) Addr() net.Addr { return l.addr }

// mdnsName returns the name this machine answers to on the local network,
// such as my-computer.local, or an empty string when there is none.
func mdnsName() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if strings.HasSuffix(host, ".local") {
		return host
	}
	if host != "" && !strings.Contains(host, ".") {
		return host + ".local"
	}
	return ""
}

// hostOnly removes the port from a Host header.
func hostOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// noStore keeps browsers from showing an old copy of a page during the test.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

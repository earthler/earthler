package machines

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/earthler/earthler/core/accounts"
	"github.com/earthler/earthler/core/index"
	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/replica"
	"github.com/earthler/earthler/core/scribe"
)

// Server is what a machine answers to the other machines of its community.
// Every machine serves its own journal; the machine that counts also takes
// new machines in and decides what is counted.
type Server struct {
	Self     uint64
	Label    string            // the name of this machine, for when it adds itself
	Secrets  string            // the secrets folder, which a new machine receives
	Index    *index.Index      // the machines index
	Scribe   *scribe.Scribe    // to write the machines that join
	Own      *journal.Journal  // the journal this machine serves
	Invites  *replica.Registry // the invitation codes, and the limit on wrong tries
	Peers    *Peers
	Accounts *accounts.Service // decides what is counted, on the machine that counts

	// Joined is called when a machine joins, so the program starts keeping
	// a copy of its journal and following it.
	Joined func(machine uint64) error

	Now func() time.Time

	joining sync.Mutex // one machine joins at a time, so numbers never repeat
	mu      sync.Mutex
	streams map[*stream]struct{}
	closed  bool
}

// Routes adds the machines' side to the server, and starts telling the
// streams of the journal about what this machine writes.
func (s *Server) Routes(mux *http.ServeMux) {
	s.streams = map[*stream]struct{}{}
	s.Scribe.Listen(s.announce)
	mux.HandleFunc("POST /machines/join", s.join)
	mux.HandleFunc("GET /machines/journal", s.authorized(s.journal))
	mux.HandleFunc("GET /machines/peers", s.authorized(s.peers))
	mux.HandleFunc("POST /machines/count/sign-up", s.authorized(s.countSignUp))
	mux.HandleFunc("POST /machines/count/role", s.authorized(s.countRole))
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func who(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type machineKey struct{}

// authorized lets a request through only with the token of another machine
// of the community, and tells the handler which machine it is.
func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Invites.Blocked(who(r), s.now()) {
			http.Error(w, "too many tries", http.StatusTooManyRequests)
			return
		}
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		machine, found, err := byToken(r.Context(), s.Index, token)
		if err != nil {
			log.Printf("machines: %v", err)
			http.Error(w, "something went wrong", http.StatusInternalServerError)
			return
		}
		if !found || machine == s.Self || token == "" {
			s.Invites.Failed(who(r), s.now())
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "a token of a machine of this community is needed", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), machineKey{}, machine)))
	}
}

// peers tells another machine the addresses this one knows, so that every
// machine can learn every other.
func (s *Server) peers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Peers.All())
}

// Close ends the streams of the journal, which never go quiet by themselves.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for st := range s.streams {
		st.drop()
		delete(s.streams, st)
	}
}

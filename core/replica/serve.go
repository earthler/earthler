package replica

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/journal"
)

// File is one file of the copy as the second machine is told about it: the
// name it has, and the size and digest of the encrypted bytes, so it can
// check what it downloaded without being able to read it.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Index tells a second machine what the copy is made of right now.
type Index struct {
	Made time.Time `json:"made"`

	// Last and Seal are the last entry the journal had when the copy was
	// made, and its seal. The copy has at least that much, so they can only
	// grow, and a second machine that sees them go back knows the main
	// machine was restored from something older.
	Last uint64 `json:"last"`
	Seal string `json:"seal"`

	// Match answers the question a second machine asks with last and seal:
	// is the history it has the same as this one. It is left out when the
	// machine asked nothing.
	Match *bool `json:"match,omitempty"`

	Manifest File   `json:"manifest"`
	Files    []File `json:"files"`
}

// Server is the main machine's side: it keeps an up to date encrypted copy
// in its own folder and hands it to the second machines that have a token.
type Server struct {
	Registry *Registry
	Journal  *journal.Journal
	DataDir  string
	Out      string // the folder where the copy for second machines is kept
	Key      copies.Key
	MinGap   time.Duration // the shortest time between two refreshes of the copy
	Now      func() time.Time

	mu    sync.Mutex
	made  time.Time
	last  uint64
	seal  string
	files []File
	sums  map[string]File // the digest of a file is worked out once: its name never changes meaning
}

// Routes adds the second machines' side to the server.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /replica/pair", s.pair)
	mux.HandleFunc("GET /replica/index", s.authorized(s.index))
	mux.HandleFunc("GET /replica/files/{name}", s.authorized(s.file))
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

// pair trades a pairing code for a token.
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	secret, err := s.Registry.Redeem(r.FormValue("code"), r.FormValue("label"), who(r), s.now())
	switch err {
	case nil:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, secret)
	case ErrTooMany:
		http.Error(w, "too many tries", http.StatusTooManyRequests)
	case ErrBadCode:
		http.Error(w, "the code is wrong, was used or has expired", http.StatusForbidden)
	default:
		log.Printf("replica: pairing: %v", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
	}
}

// authorized lets a request through only with the token of a second machine.
func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if _, good := s.Registry.Authorize(secret, who(r), s.now()); !ok || !good {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "a token of a second machine is needed", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// refresh brings the copy up to date, unless it was brought up to date a
// moment ago. The last entry is read before the copy is made, so the copy
// has at least what the index says.
func (s *Server) refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.files != nil && now.Sub(s.made) < s.MinGap {
		return nil
	}
	last, seal := s.Journal.Last()
	if _, err := copies.Copy(s.DataDir, s.Out, s.Key, now); err != nil {
		return err
	}
	files, err := s.list()
	if err != nil {
		return err
	}
	s.made, s.last, s.seal, s.files = now, last, seal.String(), files
	return nil
}

// list names every file of the copy with its size and digest.
func (s *Server) list() ([]File, error) {
	if s.sums == nil {
		s.sums = map[string]File{}
	}
	entries, err := os.ReadDir(s.Out)
	if err != nil {
		return nil, err
	}
	var files []File
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !copies.IsOurs(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		seen[name] = true
		// A file with a random name is never rewritten, so its digest is
		// kept. The manifest has one name and changes, so it is read again.
		if known, ok := s.sums[name]; ok && name != "manifest" && known.Size == info.Size() {
			files = append(files, known)
			continue
		}
		sum, err := digestOf(filepath.Join(s.Out, name))
		if err != nil {
			return nil, err
		}
		f := File{Name: name, Size: info.Size(), SHA256: sum}
		s.sums[name] = f
		files = append(files, f)
	}
	for name := range s.sums {
		if !seen[name] {
			delete(s.sums, name)
		}
	}
	return files, nil
}

func digestOf(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// index answers with what the copy is made of, after refreshing it.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if err := s.refresh(); err != nil {
		log.Printf("replica: refreshing the copy: %v", err)
		http.Error(w, "the copy could not be made", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	idx := Index{Made: s.made, Last: s.last, Seal: s.seal}
	for _, f := range s.files {
		if f.Name == "manifest" {
			idx.Manifest = f
		} else {
			idx.Files = append(idx.Files, f)
		}
	}
	s.mu.Unlock()

	if asked := r.URL.Query().Get("last"); asked != "" {
		match := s.sameHistory(asked, r.URL.Query().Get("seal"))
		idx.Match = &match
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(idx)
}

// sameHistory says whether the second machine's last entry and seal are
// those of this journal.
func (s *Server) sameHistory(last, seal string) bool {
	var n uint64
	for _, c := range last {
		if c < '0' || c > '9' || n > 1<<60 {
			return false
		}
		n = n*10 + uint64(c-'0')
	}
	mine, err := s.Journal.SealAt(n)
	return err == nil && mine.String() == seal
}

// file hands over one file of the copy.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !copies.IsOurs(name) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.Out, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

package replica

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/earthler/earthler/core/copies"
	"github.com/earthler/earthler/core/durable"
)

const (
	stateFile    = "follower.json"
	divergedDir  = "diverged"
	maxFiles     = 100000
	maxIndexSize = 64 << 20
)

// state is what a second machine remembers between two pulls. The token is
// a credential that can only pull the copy, which only the copy key reads.
type state struct {
	URL   string          `json:"url"`
	Token string          `json:"token"`
	Root  []byte          `json:"root"` // the community root, kept so the main machine can be told from a stranger
	Last  uint64          `json:"last"`
	Seal  string          `json:"seal"`
	Made  time.Time       `json:"made"`
	Held  map[string]File `json:"held"` // what is in the folder, so it is not hashed again
}

// Follower is the second machine's side: it pulls the copy into a folder.
type Follower struct {
	dir    string
	state  state
	client *http.Client
}

// Pulled says what one pull did.
type Pulled struct {
	Files      int   // files in the copy
	Downloaded int   // files that came in this pull
	Bytes      int64 // how much came
	Diverged   bool  // the main machine's history was not the one held, which was set aside
}

func (p Pulled) String() string {
	text := fmt.Sprintf("%d files, %d new, %.1f MB", p.Files, p.Downloaded, float64(p.Bytes)/1e6)
	if p.Diverged {
		text += "; the history held was not the main machine's and was set aside in " + divergedDir
	}
	return text
}

// Open returns the second machine kept in dir.
func Open(dir string) (*Follower, error) {
	raw, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return nil, fmt.Errorf("replica: %s is not a second machine, pair it first: %w", dir, err)
	}
	f := &Follower{dir: dir}
	if err := json.Unmarshal(raw, &f.state); err != nil {
		return nil, fmt.Errorf("replica: %s is damaged: %w", filepath.Join(dir, stateFile), err)
	}
	if f.state.Held == nil {
		f.state.Held = map[string]File{}
	}
	if f.client, err = pinned(f.state.Root); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Follower) save() error {
	raw, err := json.Marshal(f.state)
	if err != nil {
		return err
	}
	return durable.WriteFile(filepath.Join(f.dir, stateFile), raw, 0o600)
}

func (f *Follower) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", f.state.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.state.Token)
	res, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized {
			return nil, ErrRevoked
		}
		return nil, fmt.Errorf("replica: %s answered %d", path, res.StatusCode)
	}
	return res, nil
}

// ErrRevoked says the main machine no longer accepts this machine's token.
var ErrRevoked = errors.New("replica: the main machine refused the token: this machine was unpaired or the token was revoked")

// Pull brings the folder level with the main machine's copy. It works on a
// consistent set: files that are new come in beside the old ones, the
// manifest is put in place last, and only then are old files removed. A pull
// that is cut leaves the folder as it was.
func (f *Follower) Pull(ctx context.Context) (Pulled, error) {
	var done Pulled
	idx, err := f.index(ctx)
	if err != nil {
		return done, err
	}
	if idx.Last < f.state.Last || (idx.Match != nil && !*idx.Match) {
		if err := f.setAside(); err != nil {
			return done, err
		}
		done.Diverged = true
	}

	wanted := append([]File{idx.Manifest}, idx.Files...)
	done.Files = len(idx.Files) + 1
	got := map[string]File{}
	for _, file := range wanted {
		if !copies.IsOurs(file.Name) || file.Size < 0 {
			return done, fmt.Errorf("replica: the main machine listed a file called %q", file.Name)
		}
		if held, ok := f.state.Held[file.Name]; ok && held == file && f.has(file) {
			got[file.Name] = file
			continue
		}
		if file.Name == "manifest" {
			continue // it comes last
		}
		if err := f.fetch(ctx, file); err != nil {
			return done, err
		}
		got[file.Name] = file
		done.Downloaded++
		done.Bytes += file.Size
	}
	if held, ok := f.state.Held["manifest"]; !ok || held != idx.Manifest || !f.has(idx.Manifest) {
		if err := f.fetch(ctx, idx.Manifest); err != nil {
			return done, err
		}
		done.Downloaded++
		done.Bytes += idx.Manifest.Size
	}
	got["manifest"] = idx.Manifest
	if err := durable.SyncDir(f.dir); err != nil {
		return done, err
	}
	f.state.Held, f.state.Last, f.state.Seal, f.state.Made = got, idx.Last, idx.Seal, idx.Made
	if err := f.save(); err != nil {
		return done, err
	}
	return done, f.sweep(got)
}

func (f *Follower) index(ctx context.Context) (Index, error) {
	var idx Index
	path := "/replica/index"
	if f.state.Last > 0 {
		path += "?last=" + fmt.Sprint(f.state.Last) + "&seal=" + url.QueryEscape(f.state.Seal)
	}
	res, err := f.get(ctx, path)
	if err != nil {
		return idx, err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(io.LimitReader(res.Body, maxIndexSize)).Decode(&idx); err != nil {
		return idx, fmt.Errorf("replica: the index could not be read: %w", err)
	}
	if len(idx.Files) > maxFiles {
		return idx, errors.New("replica: the index lists too many files")
	}
	return idx, nil
}

// has says whether the folder still holds the file, with the size it should have.
func (f *Follower) has(file File) bool {
	info, err := os.Stat(filepath.Join(f.dir, file.Name))
	return err == nil && info.Size() == file.Size
}

// fetch downloads one file beside the others and puts it in place only when
// its size and digest are what the main machine said.
func (f *Follower) fetch(ctx context.Context, file File) error {
	res, err := f.get(ctx, "/replica/files/"+file.Name)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	tmp := filepath.Join(f.dir, file.Name+".tmp")
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(res.Body, file.Size+1))
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && (n != file.Size || hex.EncodeToString(h.Sum(nil)) != file.SHA256) {
		err = fmt.Errorf("replica: %s did not arrive whole, or was changed on the way", file.Name)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(f.dir, file.Name))
}

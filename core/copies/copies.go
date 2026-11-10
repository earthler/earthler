package copies

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// manifestName is the one file of a copy with a fixed name. It lists every
// file of the data folder with its digest and the random name it has in
// the copy, and it is encrypted like the rest.
const manifestName = "manifest"

type entry struct {
	Name   string `json:"name"`   // the file's name in the copy
	Digest string `json:"digest"` // SHA-256 of the content, in hex
	Size   int64  `json:"size"`
}

type manifest struct {
	Version int              `json:"version"`
	Made    time.Time        `json:"made"`
	Files   map[string]entry `json:"files"` // by path inside the data folder, with slashes
}

// Report says what a copy, a restore or a check did.
type Report struct {
	Files   int   // files in the copy
	Written int   // files written this time
	Bytes   int64 // content written, or read back, in bytes
}

func (r Report) String() string {
	return fmt.Sprintf("%d of %d files, %.1f MB", r.Written, r.Files, float64(r.Bytes)/1e6)
}

// sources lists what goes in a copy: the segments of the journal and the
// secrets. The indexes are left out, because they are rebuilt.
func sources(dataDir string) ([]string, error) {
	segments, err := filepath.Glob(filepath.Join(dataDir, "journal", "*.journal"))
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, s := range segments {
		paths = append(paths, filepath.ToSlash(filepath.Join("journal", filepath.Base(s))))
	}
	secrets := filepath.Join(dataDir, "secrets")
	err = filepath.WalkDir(secrets, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dataDir, path)
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// Copy brings the copy in dest up to date with the data folder. Only the
// files that changed since the last time are written, and what the data
// folder no longer has is removed from the copy. A first copy and a later
// one take the same call.
func Copy(dataDir, dest string, key Key, now time.Time) (Report, error) {
	var rep Report
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return rep, err
	}
	old, err := readManifest(dest, key)
	if err != nil {
		return rep, err
	}
	paths, err := sources(dataDir)
	if err != nil {
		return rep, err
	}
	next := manifest{Version: 1, Made: now, Files: map[string]entry{}}
	for _, p := range paths {
		raw, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(p)))
		if err != nil {
			return rep, err
		}
		sum := sha256.Sum256(raw)
		digest := hex.EncodeToString(sum[:])
		rep.Files++
		if prev, ok := old.Files[p]; ok && prev.Digest == digest && exists(filepath.Join(dest, prev.Name)) {
			next.Files[p] = prev
			continue
		}
		name, err := randomName()
		if err != nil {
			return rep, err
		}
		if err := writeFile(filepath.Join(dest, name), name, raw, key); err != nil {
			return rep, err
		}
		next.Files[p] = entry{Name: name, Digest: digest, Size: int64(len(raw))}
		rep.Written++
		rep.Bytes += int64(len(raw))
	}
	if err := writeManifest(dest, next, key); err != nil {
		return rep, err
	}
	return rep, sweep(dest, next)
}

// Check reads the whole copy in src and confirms every file against the
// manifest, without writing anything. It is the restore test.
func Check(src string, key Key) (Report, error) {
	return walk(src, key, func(string) (io.Writer, func() error, error) {
		return io.Discard, func() error { return nil }, nil
	})
}

// Restore writes the files of the copy in src into the data folder, which
// must be empty or absent. The indexes are rebuilt the next time the system
// starts.
func Restore(src, dataDir string, key Key) (Report, error) {
	if entries, err := os.ReadDir(dataDir); err == nil && len(entries) > 0 {
		return Report{}, fmt.Errorf("copies: %s is not empty; a copy is only restored into an empty folder", dataDir)
	}
	rep, err := walk(src, key, func(path string) (io.Writer, func() error, error) {
		full := filepath.Join(dataDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return nil, nil, err
		}
		f, err := os.OpenFile(full+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, nil, err
		}
		commit := func() error {
			if err := f.Sync(); err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			return os.Rename(full+".tmp", full)
		}
		return f, commit, nil
	})
	if err != nil {
		return rep, err
	}
	for _, dir := range []string{"journal", "secrets"} {
		if d, err := os.Open(filepath.Join(dataDir, dir)); err == nil {
			d.Sync()
			d.Close()
		}
	}
	return rep, nil
}

// walk reads every file of a copy in the order of the manifest. open gives
// where the content of one file goes and how to keep it once it was read
// whole and matched its digest.
func walk(src string, key Key, open func(path string) (io.Writer, func() error, error)) (Report, error) {
	var rep Report
	m, err := readManifest(src, key)
	if err != nil {
		return rep, err
	}
	if m.Version == 0 {
		return rep, fmt.Errorf("copies: there is no copy in %s", src)
	}
	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		e := m.Files[p]
		f, err := os.Open(filepath.Join(src, e.Name))
		if err != nil {
			return rep, fmt.Errorf("copies: the copy lost the file that holds %s", p)
		}
		w, commit, err := open(p)
		if err == nil {
			h := sha256.New()
			err = Decrypt(io.MultiWriter(w, h), e.Name, f, key)
			if err == nil && hex.EncodeToString(h.Sum(nil)) != e.Digest {
				err = fmt.Errorf("copies: %s does not match the manifest", p)
			}
			if err == nil {
				err = commit()
			}
		}
		f.Close()
		if err != nil {
			return rep, err
		}
		rep.Files++
		rep.Bytes += e.Size
	}
	return rep, nil
}

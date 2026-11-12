package copies

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func readManifest(dir string, key Key) (manifest, error) {
	var m manifest
	f, err := os.Open(filepath.Join(dir, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	defer f.Close()
	var raw bytesWriter
	if err := Decrypt(&raw, manifestName, f, key); err != nil {
		return m, fmt.Errorf("copies: the copy in %s cannot be read: it was made with another key, or it is damaged", dir)
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Version != 1 {
		return m, fmt.Errorf("copies: the manifest in %s is not one this version understands", dir)
	}
	return m, nil
}

func writeManifest(dir string, m manifest, key Key) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, manifestName), manifestName, raw, key)
}

// writeFile encrypts content into path, through a temporary file that is
// confirmed and then moved into place, so a power cut leaves either the
// old file or the new one.
func writeFile(path, name string, content []byte, key Key) error {
	f, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	err = Encrypt(f, name, bytesReader(content), key)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// ours says whether a name in the folder of a copy is one this package
// writes: the manifest, a random name, or the temporary file of either.
// Anything else in the folder belongs to someone else.
func ours(name string) bool {
	name = strings.TrimSuffix(name, ".tmp")
	if name == manifestName {
		return true
	}
	if len(name) != 32 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil && name == strings.ToLower(name)
}

// IsOurs says whether a name in the folder of a copy is a file of the copy
// itself: the manifest or a file with a random name, not a temporary file.
func IsOurs(name string) bool {
	return ours(name) && !strings.HasSuffix(name, ".tmp")
}

// foreign lists what is in dir that this package did not write.
func foreign(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var others []string
	for _, e := range entries {
		if e.IsDir() || !ours(e.Name()) {
			others = append(others, e.Name())
		}
	}
	return others, nil
}

// sweep removes from the copy whatever the manifest does not name: old
// versions of files, and leftovers of an interrupted copy. It only ever
// removes files the package wrote, so a folder shared with other things
// loses nothing it did not get from here.
func sweep(dest string, m manifest) error {
	keep := map[string]bool{manifestName: true}
	for _, e := range m.Files {
		keep[e.Name] = true
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && ours(e.Name()) && !keep[e.Name()] {
			if err := os.Remove(filepath.Join(dest, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func randomName() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type bytesWriter []byte

func (b *bytesWriter) Write(p []byte) (int, error) {
	*b = append(*b, p...)
	return len(p), nil
}

type bytesReaderT struct {
	data []byte
}

func bytesReader(data []byte) io.Reader { return &bytesReaderT{data} }

func (r *bytesReaderT) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

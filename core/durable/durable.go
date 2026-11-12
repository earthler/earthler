// Package durable writes small files so that after a power cut each one is
// either whole or not there at all.
package durable

import (
	"os"
	"path/filepath"
)

// WriteFile writes data to path. The bytes go to a file beside it, are sent
// to the disk, and only then take the place of path, which is then sent to
// the disk too. A reader never sees half a file, and a file that was
// confirmed is still there after a power cut.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return SyncDir(filepath.Dir(path))
}

// SyncDir sends a folder's list of files to the disk, so that a file that
// was just created, renamed or removed in it survives a power cut.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

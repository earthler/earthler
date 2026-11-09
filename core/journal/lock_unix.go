//go:build unix

package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const lockName = "LOCK"

// lockDir makes sure only one process has the journal open. Two writers
// would give the same number to different entries. The lock goes away by
// itself when the process ends, however it ends.
func lockDir(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("journal: %s is already open in another place", dir)
	}
	return f, nil
}

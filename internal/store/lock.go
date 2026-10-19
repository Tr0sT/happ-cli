//go:build linux || darwin

package store

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// Reload under the lock so an update timer cannot overwrite a concurrent selection.
func (s *Store) mutate(change func() error) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "state.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	fresh, err := Open(s.dir)
	if err != nil {
		return err
	}
	s.state = fresh.state
	if err := change(); err != nil {
		return err
	}
	return s.save()
}

//go:build windows

package anvil

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// renameReplace moves tmp over path, which may exist. Windows refuses with an access or sharing error while
// anything holds path open for an instant (a virus scanner, the indexer, a reader that has not closed yet), so a
// few short retries follow before giving up; the same is done by the go tool.
func renameReplace(tmp, path string) error {
	var err error
	for i := 0; i < 10; i++ {
		if err = os.Rename(tmp, path); err == nil || !transient(err) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}

func transient(err error) bool {
	var e syscall.Errno
	if !errors.As(err, &e) {
		return false
	}
	const accessDenied, sharingViolation, lockViolation = 5, 32, 33
	return e == accessDenied || e == sharingViolation || e == lockViolation
}

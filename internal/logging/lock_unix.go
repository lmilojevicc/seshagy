//go:build !windows

package logging

import (
	"os"

	"golang.org/x/sys/unix"
)

func tryLockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

func removeUnlockedLog(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer file.Close()
	if err := tryLockFile(file); err != nil {
		return true, nil
	}
	defer unlockFile(file) //nolint:errcheck // removal result is the actionable retention outcome
	return false, os.Remove(path)
}

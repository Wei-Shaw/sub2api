//go:build !darwin && !linux

package deployer

import (
	"errors"
	"os"
)

var errProcessLockUnsupported = errors.New("deployer process locking is supported only on Linux and macOS")

func lockFile(_ *os.File) error {
	return errProcessLockUnsupported
}

func unlockFile(_ *os.File) error {
	return nil
}

func isConnectionRefused(_ error) bool {
	return false
}

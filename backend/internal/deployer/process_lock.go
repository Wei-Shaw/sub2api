package deployer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrProcessLocked = errors.New("another deployer process holds the state lock")

type ProcessLock struct {
	file *os.File
}

func AcquireProcessLock(statePath string) (*ProcessLock, error) {
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create deployer state directory: %w", err)
	}
	path := filepath.Join(dir, "deployer.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open deployer process lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrProcessLocked) {
			return nil, fmt.Errorf("%w: %s", ErrProcessLocked, path)
		}
		return nil, fmt.Errorf("lock deployer process file: %w", err)
	}
	return &ProcessLock{file: file}, nil
}

func (l *ProcessLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	unlockErr := unlockFile(file)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}

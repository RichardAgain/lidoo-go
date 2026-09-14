package files

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

const stateLockPath = ".lidoo.json.lock"

type StateLock struct {
	file *os.File
}

func AcquireStateLock() (*StateLock, error) {
	file, err := os.OpenFile(stateLockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("workspace is busy: another lidoo command is changing state")
		}
		return nil, fmt.Errorf("lock workspace: %w", err)
	}
	return &StateLock{file: file}, nil
}

func (lock *StateLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return fmt.Errorf("unlock workspace: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close workspace lock: %w", closeErr)
	}
	return nil
}

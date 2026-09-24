package app

import (
	"context"
	"fmt"

	"lidoo/internal/files"
)

func (s *Service) withWorkspaceOperation(ctx context.Context, save bool, operation func(files.State) error) (err error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	lock, err := files.AcquireStateLock()
	if err != nil {
		return err
	}
	defer func() {
		closeErr := lock.Close()
		if closeErr == nil {
			return
		}
		if err == nil {
			err = closeErr
			return
		}
		err = fmt.Errorf("%w; additionally failed to release workspace lock: %v", err, closeErr)
	}()

	state, err := files.ReadState()
	if err != nil {
		return fmt.Errorf("read workspace state: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := operation(state); err != nil {
		return err
	}
	if save {
		if err := files.SaveState(state); err != nil {
			return fmt.Errorf("save workspace state: %w", err)
		}
	}
	return nil
}

package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"lidoo/internal/docker"
)

type ProfileInvalidationKind uint8

const (
	ProfileRuntimeChanged ProfileInvalidationKind = iota
	ProfileRuntimeUnavailable
)

type ProfileInvalidation struct {
	ProfileName string
	Kind        ProfileInvalidationKind
}

func (s *Service) WatchProfileInvalidations(ctx context.Context) (<-chan ProfileInvalidation, <-chan error) {
	if ctx == nil {
		ctx = context.Background()
	}
	invalidations := make(chan ProfileInvalidation, 16)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(invalidations)
		defer close(errorsOut)
		const maxReconnects = 3
		var lastErr error
		for attempt := 0; attempt <= maxReconnects; attempt++ {
			rawEvents, rawErrors := docker.WatchContainerEvents(ctx)
			lastErr = forwardInvalidations(ctx, invalidations, rawEvents, rawErrors)
			if ctx.Err() != nil {
				return
			}
			if lastErr == nil {
				lastErr = errors.New("Docker event stream disconnected")
			}
			if attempt == maxReconnects {
				select {
				case errorsOut <- lastErr:
				case <-ctx.Done():
				}
				return
			}
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
	}()
	return invalidations, errorsOut
}

func forwardInvalidations(ctx context.Context, output chan<- ProfileInvalidation, input <-chan docker.ContainerEvent, inputErrors <-chan error) error {
	pending := make(map[string]ProfileInvalidationKind)
	var timer *time.Timer
	var timerChannel <-chan time.Time
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		names := make([]string, 0, len(pending))
		for name := range pending {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			select {
			case output <- ProfileInvalidation{ProfileName: name, Kind: pending[name]}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		clear(pending)
		return nil
	}
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerChannel = nil
	}

	for input != nil || inputErrors != nil {
		select {
		case event, ok := <-input:
			if !ok {
				input = nil
				if inputErrors == nil {
					stopTimer()
					if err := flush(); err != nil {
						return err
					}
					return errors.New("Docker event stream disconnected")
				}
				continue
			}
			invalidation, ok := invalidationForEvent(event)
			if !ok {
				continue
			}
			pending[invalidation.ProfileName] = invalidation.Kind
			if timer == nil {
				timer = time.NewTimer(150 * time.Millisecond)
				timerChannel = timer.C
			}
		case eventErr, ok := <-inputErrors:
			if !ok {
				inputErrors = nil
				if input == nil {
					stopTimer()
					if err := flush(); err != nil {
						return err
					}
					return errors.New("Docker event stream disconnected")
				}
				continue
			}
			if eventErr != nil {
				stopTimer()
				if err := flush(); err != nil {
					return err
				}
				return eventErr
			}
		case <-timerChannel:
			timer = nil
			timerChannel = nil
			if err := flush(); err != nil {
				return err
			}
		case <-ctx.Done():
			stopTimer()
			return ctx.Err()
		}
	}
	stopTimer()
	if err := flush(); err != nil {
		return err
	}
	return errors.New("Docker event stream disconnected")
}

func invalidationForEvent(event docker.ContainerEvent) (ProfileInvalidation, bool) {
	if strings.HasPrefix(event.Action, "exec_") {
		return ProfileInvalidation{}, false
	}
	switch event.Action {
	case "create", "start", "restart", "unpause":
		return ProfileInvalidation{ProfileName: event.ProfileName, Kind: ProfileRuntimeChanged}, true
	case "stop", "die", "destroy", "pause":
		return ProfileInvalidation{ProfileName: event.ProfileName, Kind: ProfileRuntimeUnavailable}, true
	default:
		return ProfileInvalidation{}, false
	}
}

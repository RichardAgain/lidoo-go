package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type ContainerEvent struct {
	ProfileName string
	Action      string
}

type rawContainerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Status string `json:"status"`
	Actor  struct {
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
}

func ParseContainerEvent(data []byte) (ContainerEvent, error) {
	var raw rawContainerEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return ContainerEvent{}, err
	}
	if raw.Type != "" && raw.Type != "container" {
		return ContainerEvent{}, errors.New("Docker event is not a container event")
	}
	if raw.Actor.Attributes["io.lidoo.managed"] != "true" {
		return ContainerEvent{}, errors.New("Docker event is not a managed profile")
	}
	name := raw.Actor.Attributes["io.lidoo.name"]
	if err := ValidateProfileName(name); err != nil {
		return ContainerEvent{}, fmt.Errorf("invalid profile in Docker event: %w", err)
	}
	action := raw.Action
	if action == "" {
		action = raw.Status
	}
	action = strings.ToLower(strings.TrimSpace(strings.SplitN(action, ":", 2)[0]))
	if action == "" {
		return ContainerEvent{}, errors.New("Docker event has no action")
	}
	return ContainerEvent{ProfileName: name, Action: action}, nil
}

func WatchContainerEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	if ctx == nil {
		ctx = context.Background()
	}
	events := make(chan ContainerEvent, 16)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errorsOut)
		command := exec.CommandContext(ctx,
			"docker", "events",
			"--filter", "type=container",
			"--filter", "label=io.lidoo.managed=true",
			"--format", "{{json .}}",
		)
		stdout, err := command.StdoutPipe()
		if err != nil {
			errorsOut <- err
			return
		}
		command.Stderr = io.Discard
		if err := command.Start(); err != nil {
			errorsOut <- err
			return
		}

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			event, err := ParseContainerEvent(scanner.Bytes())
			if err != nil {
				continue
			}
			select {
			case events <- event:
			case <-ctx.Done():
				_ = command.Wait()
				return
			}
		}
		var streamErr error
		if err := scanner.Err(); err != nil {
			streamErr = err
		}
		if err := command.Wait(); err != nil && streamErr == nil {
			streamErr = err
		}
		if streamErr != nil && ctx.Err() == nil {
			errorsOut <- streamErr
		}
	}()
	return events, errorsOut
}

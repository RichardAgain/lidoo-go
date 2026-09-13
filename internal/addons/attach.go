package addons

import (
	"errors"
	"fmt"
	"strings"

	"lidoo/internal/files"
)

func AttachToContainer(container string, names []string, state files.State) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}
	for _, name := range names {
		if !validAddonName(name) {
			return fmt.Errorf("invalid addon name %q", name)
		}
	}
	if err := files.AddContainer(state, container); err != nil {
		return fmt.Errorf("add container %q to state: %w", container, err)
	}
	if err := files.AddAddonsToContainer(state, container, names); err != nil {
		return fmt.Errorf("update container %q: %w", container, err)
	}
	return nil
}

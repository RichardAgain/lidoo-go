package addons

import (
	"errors"
	"fmt"
	"strings"

	"lidoo/internal/files"
)

func DetachFromContainer(container string, names []string, state files.State) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}
	for _, name := range names {
		if !validAddonName(name) {
			return fmt.Errorf("invalid addon name %q", name)
		}
	}
	if err := files.RemoveAddonsFromContainer(state, container, names); err != nil {
		return fmt.Errorf("update container %q: %w", container, err)
	}
	return nil
}

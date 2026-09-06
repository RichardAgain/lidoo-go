package docker

import (
	"errors"
	"fmt"
)

func Stop(name string) error {
	if name == "" {
		return errors.New("stop requires name")
	}

	containers, err := containerIDs("label="+containerNameLabel+"="+name, false)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no running container with name %q", name)
	}
	for _, container := range containers {
		if err := dockerQuiet("stop", container); err != nil {
			return fmt.Errorf("stop container %s: %w", container, err)
		}
	}
	return nil
}

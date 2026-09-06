package docker

import (
	"fmt"
	"io"
	"os/exec"
)

const FilestoreVolumeName = "lidoo-filestore-data"

func EnsureVolume() error {
	inspect := exec.Command("docker", "volume", "inspect", FilestoreVolumeName)
	inspect.Stdout = io.Discard
	inspect.Stderr = io.Discard
	if inspect.Run() == nil {
		return nil
	}

	create := exec.Command("docker", "volume", "create", FilestoreVolumeName)
	create.Stdout = io.Discard
	create.Stderr = io.Discard
	if err := create.Run(); err != nil {
		return fmt.Errorf("create Docker volume %q: %w", FilestoreVolumeName, err)
	}
	return nil
}

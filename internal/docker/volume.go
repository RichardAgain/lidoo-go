package docker

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

const FilestoreVolumeName = "lidoo-filestore-data"

func EnsureVolume() error {
	return EnsureVolumeWithContext(context.Background())
}

func EnsureVolumeWithContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	inspect := exec.CommandContext(ctx, "docker", "volume", "inspect", FilestoreVolumeName)
	inspect.Stdout = io.Discard
	inspect.Stderr = io.Discard
	if inspect.Run() == nil {
		return nil
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}

	create := exec.CommandContext(ctx, "docker", "volume", "create", FilestoreVolumeName)
	create.Stdout = io.Discard
	create.Stderr = io.Discard
	if err := create.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf("create Docker volume %q: %w", FilestoreVolumeName, err)
	}
	return nil
}

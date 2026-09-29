package docker

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func filestoreVolumeName(name string) string {
	return "lidoo-filestore-" + name
}

func EnsureVolumeWithContext(ctx context.Context, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	volume := filestoreVolumeName(name)
	inspect := exec.CommandContext(ctx, "docker", "volume", "inspect", volume)
	inspect.Stdout = io.Discard
	inspect.Stderr = io.Discard
	if inspect.Run() == nil {
		return nil
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}

	create := exec.CommandContext(ctx, "docker", "volume", "create", volume)
	create.Stdout = io.Discard
	create.Stderr = io.Discard
	if err := create.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf("create Docker volume %q: %w", volume, err)
	}
	return nil
}

func filestoreVolumeExists(name string, options CommandOptions) (bool, error) {
	volume := filestoreVolumeName(name)
	output, err := dockerOutputWithOptions(options, "volume", "ls", "--quiet", "--filter", "name="+volume)
	if err != nil {
		return false, fmt.Errorf("find Docker volume %q: %w", volume, err)
	}
	for _, found := range strings.Fields(string(output)) {
		if found == volume {
			return true, nil
		}
	}
	return false, nil
}

func removeFilestoreVolume(name string, options CommandOptions) error {
	exists, err := filestoreVolumeExists(name, options)
	if err != nil || !exists {
		return err
	}
	volume := filestoreVolumeName(name)
	if err := dockerQuietWithOptions(options, "volume", "rm", volume); err != nil {
		return fmt.Errorf("remove Docker volume %q: %w", volume, err)
	}
	return nil
}

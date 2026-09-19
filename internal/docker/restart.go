package docker

import "fmt"

func Restart(name string) error {
	return RestartWithOptions(name, CommandOptions{})
}

func RestartWithOptions(name string, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	if name == "" {
		return fmt.Errorf("restart requires name")
	}

	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, false, options)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no running container with name %q", name)
	}
	if err := networkExistsWithContext(options.Context); err != nil {
		return err
	}
	for _, container := range containers {
		if err := dockerQuietWithOptions(options, "restart", container); err != nil {
			return fmt.Errorf("restart container %s: %w", container, err)
		}
	}
	return nil
}

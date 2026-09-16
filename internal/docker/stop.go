package docker

import "fmt"

func Stop(name string) error {
	return StopWithOptions(name, CommandOptions{})
}

func StopWithOptions(name string, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	if name == "" {
		return fmt.Errorf("stop requires name")
	}

	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, false, options)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no running container with name %q", name)
	}
	for _, container := range containers {
		if err := dockerQuietWithOptions(options, "stop", container); err != nil {
			return fmt.Errorf("stop container %s: %w", container, err)
		}
	}
	return nil
}

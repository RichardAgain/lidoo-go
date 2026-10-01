package docker

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContainerRuntimeFlags reproduces one container's runtime as `docker run`
// flags, image excluded: its network, environment, mounts and host aliases.
//
// The migration uses it to run a one-off Odoo in the profile's own runtime.
// The profile container's main process is Odoo itself, so a second Odoo
// upgrading the same database conflicts with it; stopping the profile and
// reusing its runtime in a throwaway container keeps the profile's image,
// addons and filestore without inventing a second definition of them.
func ContainerRuntimeFlags(container string, options CommandOptions) ([]string, string, error) {
	options = normalizeCommandOptions(options)
	format := strings.Join([]string{
		"{{.Config.Image}}",
		"{{json .Config.Env}}",
		"{{json .Mounts}}",
		"{{json .NetworkSettings.Networks}}",
		"{{json .HostConfig.ExtraHosts}}",
	}, "\t")
	output, err := dockerOutputWithOptions(options, "inspect", "--format", format, container)
	if err != nil {
		return nil, "", fmt.Errorf("inspect container %q: %w", container, err)
	}
	return runtimeFlagsFromInspect(container, string(output))
}

func runtimeFlagsFromInspect(container, output string) ([]string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(output), "\t", 5)
	if len(parts) != 5 {
		return nil, "", fmt.Errorf("unexpected docker inspect output for container %q", container)
	}

	var environment []string
	if err := json.Unmarshal([]byte(parts[1]), &environment); err != nil {
		return nil, "", fmt.Errorf("parse container %q environment: %w", container, err)
	}
	var mounts []struct {
		Source      string
		Destination string
	}
	if err := json.Unmarshal([]byte(parts[2]), &mounts); err != nil {
		return nil, "", fmt.Errorf("parse container %q mounts: %w", container, err)
	}
	var networks map[string]json.RawMessage
	if err := json.Unmarshal([]byte(parts[3]), &networks); err != nil {
		return nil, "", fmt.Errorf("parse container %q networks: %w", container, err)
	}

	var extraHosts []string
	if err := json.Unmarshal([]byte(parts[4]), &extraHosts); err != nil {
		return nil, "", fmt.Errorf("parse container %q host aliases: %w", container, err)
	}

	network := networkName
	for name := range networks {
		network = name
		break
	}

	flags := []string{"--network", network}
	for _, host := range extraHosts {
		flags = append(flags, "--add-host", host)
	}
	for _, value := range environment {
		flags = append(flags, "--env", value)
	}
	for _, mount := range mounts {
		flags = append(flags, "-v", mount.Source+":"+mount.Destination)
	}
	return flags, parts[0], nil
}

// RunOneOff runs *command* in a throwaway container built from *container*'s
// runtime, with *extraMounts* appended as `-v` values.
//
// The image entrypoint is bypassed: it appends the PostgreSQL connection
// arguments derived from the environment at the end of the command line, which
// duplicates the ones the caller places explicitly and which the `db`
// subcommand rejects in that position.
func RunOneOff(container string, extraMounts, command []string, options CommandOptions) error {
	flags, image, err := ContainerRuntimeFlags(container, options)
	if err != nil {
		return err
	}
	args := append([]string{"run", "--rm"}, flags...)
	for _, mount := range extraMounts {
		args = append(args, "-v", mount)
	}
	args = append(args, "--entrypoint", "odoo", image)
	args = append(args, command...)
	return dockerQuietWithOptions(options, args...)
}

// StartWithOptions starts the profile's container, stopped or not.
func StartWithOptions(name string, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	if name == "" {
		return fmt.Errorf("start requires name")
	}
	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, true, options)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no container with name %q", name)
	}
	for _, container := range containers {
		if err := dockerQuietWithOptions(options, "start", container); err != nil {
			return fmt.Errorf("start container %s: %w", container, err)
		}
	}
	return nil
}

package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"lidoo/internal/profile"
)

const (
	networkName        = "lidoo-net"
	databaseEnvFile    = ".env"
	containerNameLabel = "io.lidoo.name"
)

// CommandOptions carries the process context and terminal streams for a
// Docker operation. Application callers can provide buffers instead of
// writing into Bubble Tea's terminal.
type CommandOptions struct {
	Context context.Context
	Stdout  io.Writer
	Stderr  io.Writer
}

func normalizeCommandOptions(options CommandOptions) CommandOptions {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	return options
}

func containerIDs(filter string, all bool) ([]string, error) {
	return containerIDsWithOptions(filter, all, CommandOptions{})
}

func containerIDsWithOptions(filter string, all bool, options CommandOptions) ([]string, error) {
	options = normalizeCommandOptions(options)
	args := []string{"ps"}
	if all {
		args = append(args, "--all")
	}
	args = append(args, "--quiet", "--filter", filter)

	output, err := dockerOutputWithOptions(options, args...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}

func findContainerByName(name string) (bool, error) {
	return findContainerByNameWithOptions(name, CommandOptions{})
}

func findContainerByNameWithOptions(name string, options CommandOptions) (bool, error) {
	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, true, options)
	if err != nil {
		return false, fmt.Errorf("find container with label %q: %w", name, err)
	}
	return len(containers) > 0, nil
}

func ProfileExists(name string) (bool, error) {
	return ProfileExistsWithOptions(name, CommandOptions{})
}

func ProfileExistsWithOptions(name string, options CommandOptions) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("profile name cannot be empty")
	}
	if err := profile.ValidateName(name); err != nil {
		return false, err
	}
	return findContainerByNameWithOptions(name, options)
}

func containerIsRunning(name string) (bool, error) {
	return containerIsRunningWithOptions(name, CommandOptions{})
}

func containerIsRunningWithOptions(name string, options CommandOptions) (bool, error) {
	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, false, options)
	if err != nil {
		return false, fmt.Errorf("check container with label %q: %w", name, err)
	}
	return len(containers) > 0, nil
}

func ProfileState(name string) (string, bool, error) {
	if err := profile.ValidateName(name); err != nil {
		return "", false, err
	}
	containers, err := containerIDs("label="+containerNameLabel+"="+name, true)
	if err != nil {
		return "", false, fmt.Errorf("find profile with label %q: %w", name, err)
	}
	if len(containers) == 0 {
		return "", false, nil
	}
	output, err := dockerOutput("inspect", "--format", "{{.State.Status}}", containers[0])
	if err != nil {
		return "", true, fmt.Errorf("inspect profile %q: %w", name, err)
	}
	return strings.TrimSpace(string(output)), true, nil
}

func RequireRunningProfile(name string) (string, error) {
	exists, err := findContainerByName(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("no profile with name %q", name)
	}
	running, err := containerIsRunning(name)
	if err != nil {
		return "", err
	}
	if !running {
		return "", fmt.Errorf("profile %q is not running", name)
	}
	return "lidoo-" + name, nil
}

func networkExists() error {
	return networkExistsWithContext(context.Background())
}

func networkExistsWithContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "docker", "network", "inspect", networkName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf("Docker network %q does not exist", networkName)
	}
	return nil
}

func docker(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerQuiet(args ...string) error {
	return dockerQuietWithOptions(CommandOptions{}, args...)
}

func dockerQuietWithOptions(options CommandOptions, args ...string) error {
	options = normalizeCommandOptions(options)
	cmd := exec.CommandContext(options.Context, "docker", args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if output.Len() > 0 {
			fmt.Fprint(options.Stderr, output.String())
		}
		if contextErr := options.Context.Err(); contextErr != nil {
			return contextErr
		}
		return err
	}
	return nil
}

func dockerCommandAvailable(args ...string) bool {
	return dockerCommandAvailableWithContext(context.Background(), args...)
}

func dockerCommandAvailableWithContext(ctx context.Context, args ...string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func dockerOutput(args ...string) ([]byte, error) {
	return dockerOutputWithOptions(CommandOptions{}, args...)
}

func dockerOutputWithOptions(options CommandOptions, args ...string) ([]byte, error) {
	options = normalizeCommandOptions(options)
	cmd := exec.CommandContext(options.Context, "docker", args...)
	cmd.Stderr = options.Stderr
	output, err := cmd.Output()
	if err != nil {
		if contextErr := options.Context.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	return output, nil
}

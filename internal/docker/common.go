package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	networkName        = "lidoo-net"
	databaseEnvFile    = ".env"
	containerNameLabel = "io.lidoo.name"
)

func containerIDs(filter string, all bool) ([]string, error) {
	args := []string{"ps"}
	if all {
		args = append(args, "--all")
	}
	args = append(args, "--quiet", "--filter", filter)

	output, err := dockerOutput(args...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}

func findContainerByName(name string) (bool, error) {
	containers, err := containerIDs("label="+containerNameLabel+"="+name, true)
	if err != nil {
		return false, fmt.Errorf("find container with label %q: %w", name, err)
	}
	return len(containers) > 0, nil
}

func containerIsRunning(name string) (bool, error) {
	containers, err := containerIDs("label="+containerNameLabel+"="+name, false)
	if err != nil {
		return false, fmt.Errorf("check container with label %q: %w", name, err)
	}
	return len(containers) > 0, nil
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
	cmd := exec.Command("docker", "network", "inspect", networkName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
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
	cmd := exec.Command("docker", args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if output.Len() > 0 {
			fmt.Fprint(os.Stderr, output.String())
		}
		return err
	}
	return nil
}

func dockerCommandAvailable(args ...string) bool {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func dockerOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

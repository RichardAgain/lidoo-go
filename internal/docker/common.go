package docker

import (
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

func containerHasTraefikRoute(containerName, profileName string) (bool, error) {
	format := fmt.Sprintf(`{{index .Config.Labels "traefik.http.routers.%s.rule"}}`, profileName)
	output, err := dockerOutput("inspect", "--format", format, containerName)
	if err != nil {
		return false, fmt.Errorf("inspect container %q routing: %w", containerName, err)
	}
	want := "Host(`" + profileHostname(profileName) + "`)"
	return strings.TrimSpace(string(output)) == want, nil
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

func dockerOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

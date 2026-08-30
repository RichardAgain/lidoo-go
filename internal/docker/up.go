package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	hostPortStart = 49000
	hostPortEnd   = 49999
	odooPort      = 8069
)

var odooVersion = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

func Up(args []string) error {
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "container name")
	version := flags.String("version", "", "Odoo version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("up does not accept positional arguments")
	}
	if *name == "" || *version == "" {
		return errors.New("up requires --name and --version")
	}
	if !odooVersion.MatchString(*version) {
		return fmt.Errorf("invalid Odoo version %q", *version)
	}

	containerName := "lidoo-" + *name
	exists, err := findContainerByName(containerName)
	if err != nil {
		return err
	}
	if exists {
		running, err := containerIsRunning(containerName)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("container %q already exists", containerName)
		}
		if err := docker("start", containerName); err != nil {
			return fmt.Errorf("start container %q: %w", containerName, err)
		}
		return reportContainerPort(containerName)
	}

	dockerfile := filepath.Join("docker", "Dockerfile."+*version)
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("Dockerfile for Odoo %s not found: %s", *version, dockerfile)
	}

	if _, err := os.Stat(databaseEnvFile); err != nil {
		return fmt.Errorf("%q not found: %w", databaseEnvFile, err)
	}
	if err := networkExists(); err != nil {
		return err
	}

	image := "lidoo-odoo:" + *version
	if err := docker("build", "--file", dockerfile, "--tag", image, "."); err != nil {
		return fmt.Errorf("build Odoo image: %w", err)
	}

	hostPort, err := findAvailableHostPort()
	if err != nil {
		return err
	}
	if err := docker(
		"run", "--detach",
		"--name", containerName,
		"--network", networkName,
		"--env-file", databaseEnvFile,
		"--label", containerNameLabel+"="+*name,
		"-p", fmt.Sprintf("%d:%d", hostPort, odooPort),
		image,
	); err != nil {
		return fmt.Errorf("create Odoo container: %w", err)
	}
	return reportContainerPort(containerName)
}

func findAvailableHostPort() (int, error) {
	output, err := dockerOutput("ps", "--all", "--quiet")
	if err != nil {
		return 0, fmt.Errorf("find Docker containers: %w", err)
	}

	used := make(map[int]bool)
	format := `{{if .State.Running}}{{range $port, $bindings := .NetworkSettings.Ports}}{{range $binding := $bindings}}{{println $binding.HostPort}}{{end}}{{end}}{{else}}{{range $port, $bindings := .HostConfig.PortBindings}}{{range $binding := $bindings}}{{println $binding.HostPort}}{{end}}{{end}}{{end}}`
	for _, containerID := range strings.Fields(string(output)) {
		output, err := dockerOutput("inspect", "--format", format, containerID)
		if err != nil {
			return 0, fmt.Errorf("inspect Docker container %q ports: %w", containerID, err)
		}
		for _, binding := range strings.Fields(string(output)) {
			if err := markUsedHostPorts(used, binding); err != nil {
				return 0, fmt.Errorf("inspect Docker container %q port %q: %w", containerID, binding, err)
			}
		}
	}

	for port := hostPortStart; port <= hostPortEnd; port++ {
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available host port in range %d-%d", hostPortStart, hostPortEnd)
}

func markUsedHostPorts(used map[int]bool, binding string) error {
	parts := strings.Split(binding, "-")
	if len(parts) > 2 || parts[0] == "" {
		return fmt.Errorf("invalid host port %q", binding)
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid host port %q: %w", binding, err)
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil || end < start {
			return fmt.Errorf("invalid host port range %q", binding)
		}
	}

	for port := start; port <= end; port++ {
		if port >= hostPortStart && port <= hostPortEnd {
			used[port] = true
		}
	}
	return nil
}

func reportContainerPort(containerName string) error {
	format := fmt.Sprintf(`{{(index (index .NetworkSettings.Ports "%d/tcp") 0).HostPort}}`, odooPort)
	output, err := dockerOutput("inspect", "--format", format, containerName)
	if err != nil {
		return fmt.Errorf("inspect container %q port: %w", containerName, err)
	}

	port := strings.TrimSpace(string(output))
	if port == "" {
		return fmt.Errorf("container %q has no published Odoo port", containerName)
	}
	fmt.Printf("container running on port %s\n", port)
	return nil
}

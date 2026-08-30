package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	if err := docker(
		"run", "--detach",
		"--name", containerName,
		"--network", networkName,
		"--env-file", databaseEnvFile,
		"--label", containerNameLabel+"="+*name,
		"-p", fmt.Sprintf("%d-%d:%d", hostPortStart, hostPortEnd, odooPort),
		image,
	); err != nil {
		return fmt.Errorf("create Odoo container: %w", err)
	}
	return reportContainerPort(containerName)
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

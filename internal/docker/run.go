package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/hosts"
)

const (
	odooPort      = 8069
	profileDomain = "lidoo.test"
)

var (
	odooVersion        = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
	profileNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
)

func validateProfileName(name string) error {
	if len(name) > 63 || !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use 1-63 lowercase letters, numbers, and hyphens", name)
	}
	return nil
}

func profileHostname(name string) string {
	return name + "." + profileDomain
}

func traefikLabels(name string) []string {
	hostname := profileHostname(name)
	return []string{
		"traefik.enable=true",
		"traefik.docker.network=" + networkName,
		"traefik.http.routers." + name + ".rule=Host(`" + hostname + "`)",
		"traefik.http.routers." + name + ".entrypoints=web",
		"traefik.http.routers." + name + ".service=" + name,
		"traefik.http.services." + name + ".loadbalancer.server.port=" + fmt.Sprint(odooPort),
	}
}

func buildImageArgs(dockerfile, image string, buildx bool) []string {
	if buildx {
		return []string{"buildx", "build", "--load", "--file", dockerfile, "--tag", image, "."}
	}
	return []string{"build", "--file", dockerfile, "--tag", image, "."}
}

func buildImage(dockerfile, image string) error {
	buildx := dockerCommandAvailable("buildx", "version")
	return dockerQuiet(buildImageArgs(dockerfile, image, buildx)...)
}

func Run(args []string, state files.State) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "container name")
	version := flags.String("version", "", "Odoo version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("run does not accept positional arguments")
	}
	if *name == "" || *version == "" {
		return errors.New("run requires --name and --version")
	}
	if !odooVersion.MatchString(*version) {
		return fmt.Errorf("invalid Odoo version %q", *version)
	}

	addonNames, err := files.ContainerAddons(state, *name)
	if err != nil {
		return err
	}
	prefix, err := files.ContainerPrefix(state, *name)
	if err != nil {
		return err
	}

	containerName := "lidoo-" + *name
	exists, err := findContainerByName(*name)
	if err != nil {
		return err
	}
	if exists {
		routingConfigured, err := containerHasTraefikRoute(containerName, *name)
		if err != nil {
			return err
		}
		if !routingConfigured {
			return fmt.Errorf("container %q uses legacy port routing; remove and recreate it before opening %s", containerName, profileHostname(*name))
		}

		if _, err := hosts.Ensure(profileHostname(*name)); err != nil {
			return err
		}
		running, err := containerIsRunning(*name)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("container %q already exists", containerName)
		}
		if err := dockerQuiet("start", containerName); err != nil {
			return fmt.Errorf("start container %q: %w", containerName, err)
		}
		if err := files.AddContainer(state, *name); err != nil {
			return fmt.Errorf("update state: %w", err)
		}
		return reportContainerURL(*name)
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
	fmt.Printf("building Odoo %s image\n", *version)
	if err := buildImage(dockerfile, image); err != nil {
		return fmt.Errorf("build Odoo image: %w", err)
	}

	hostname := profileHostname(*name)
	hostChanged, err := hosts.Ensure(hostname)
	if err != nil {
		return err
	}

	containerArgs := []string{
		"run", "--detach",
		"--name", containerName,
		"--network", networkName,
		"--env-file", databaseEnvFile,
		"--env", "HOST=lidoo-postgres",
		"--env", "PORT=5432",
		"--label", containerNameLabel + "=" + *name,
	}

	addonPaths := []string{"/usr/lib/python3/dist-packages/odoo/addons"}
	for _, addonName := range addonNames {
		containerArgs = append(containerArgs,
			"-v", "./"+filepath.ToSlash(filepath.Join("addons", addonName))+":/opt/addons/"+addonName,
		)
		addonPaths = append(addonPaths, "/opt/addons/"+addonName)
	}

	for _, label := range traefikLabels(*name) {
		containerArgs = append(containerArgs, "--label", label)
	}
	containerArgs = append(containerArgs, image, "odoo", "--dev=all")
	if len(addonNames) > 0 {
		containerArgs = append(containerArgs, "--addons-path="+strings.Join(addonPaths, ","))
	}
	containerArgs = append(containerArgs, databaseFilter(prefix))
	fmt.Printf("starting profile %q\n", *name)
	if err := dockerQuiet(containerArgs...); err != nil {
		if hostChanged {
			_ = hosts.Remove(hostname)
		}
		return fmt.Errorf("create Odoo container: %w", err)
	}
	if err := files.AddContainer(state, *name); err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	return reportContainerURL(*name)
}

func databaseFilter(prefix string) string {
	return "--db-filter=^" + regexp.QuoteMeta(prefix) + ".*$"
}

func reportContainerURL(name string) error {
	hostname := profileHostname(name)
	fmt.Printf("profile running at http://%s\n", hostname)
	return nil
}

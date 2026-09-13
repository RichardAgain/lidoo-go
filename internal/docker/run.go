package docker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
	"lidoo/internal/proxy"
)

var odooVersion = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

func ValidateProfileName(name string) error {
	return profile.ValidateName(name)
}

func profileHostname(name string) string {
	return profile.Hostname(name)
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

func Run(name, version string, state files.State) error {
	if name == "" {
		return errors.New("run requires name")
	}
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	storedVersion, err := files.ContainerVersion(state, name)
	if err != nil {
		return err
	}
	selectedVersion := version
	if storedVersion != nil {
		selectedVersion = *storedVersion
		if version != "" {
			fmt.Fprintf(os.Stderr, "warning: container %q uses version %q; ignoring --version %q\n", name, selectedVersion, version)
		}
	} else if version == "" {
		return fmt.Errorf("run requires --version because container %q has no stored version", name)
	}
	if !odooVersion.MatchString(selectedVersion) {
		return fmt.Errorf("invalid Odoo version %q", selectedVersion)
	}

	addonNames, err := files.ContainerAddons(state, name)
	if err != nil {
		return err
	}
	prefix, err := files.ContainerPrefix(state, name)
	if err != nil {
		return err
	}

	containerName := profile.ContainerName(name)
	exists, err := findContainerByName(name)
	if err != nil {
		return err
	}
	if exists {
		previousState := cloneState(state)
		running, err := containerIsRunning(name)
		if err != nil {
			return err
		}
		if running {
			if err := files.AddContainer(state, name); err != nil {
				restoreState(state, previousState)
				return fmt.Errorf("update state: %w", err)
			}
			if err := proxy.Sync(state); err != nil {
				restoreState(state, previousState)
				return fmt.Errorf("synchronize Caddy routing: %w", err)
			}
			return fmt.Errorf("container %q already exists", containerName)
		}
		if err := dockerQuiet("start", containerName); err != nil {
			return fmt.Errorf("start container %q: %w", containerName, err)
		}
		if err := files.AddContainer(state, name); err != nil {
			_ = dockerQuiet("stop", containerName)
			restoreState(state, previousState)
			return fmt.Errorf("update state: %w", err)
		}
		if err := proxy.Sync(state); err != nil {
			_ = dockerQuiet("stop", containerName)
			restoreState(state, previousState)
			return fmt.Errorf("synchronize Caddy routing: %w", err)
		}
		return reportContainerURL(name)
	}

	dockerfile := filepath.Join("docker", "Dockerfile."+selectedVersion)
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("Dockerfile for Odoo %s not found: %s", selectedVersion, dockerfile)
	}

	if _, err := os.Stat(databaseEnvFile); err != nil {
		return fmt.Errorf("%q not found: %w", databaseEnvFile, err)
	}
	if err := networkExists(); err != nil {
		return err
	}
	if err := EnsureVolume(); err != nil {
		return fmt.Errorf("ensure filestore volume: %w", err)
	}

	image := "lidoo-odoo:" + selectedVersion
	fmt.Printf("building Odoo %s image\n", selectedVersion)
	if err := buildImage(dockerfile, image); err != nil {
		return fmt.Errorf("build Odoo image: %w", err)
	}

	containerArgs := []string{
		"run", "--detach",
		"--name", containerName,
		"--network", networkName,
		"--env-file", databaseEnvFile,
		"--env", "HOST=lidoo-postgres",
		"--env", "PORT=5432",
		"--label", containerNameLabel + "=" + name,
		"-v", FilestoreVolumeName + ":/var/lib/odoo",
	}

	addonPaths := []string{"/usr/lib/python3/dist-packages/odoo/addons"}
	for _, addonName := range addonNames {
		containerArgs = append(containerArgs,
			"-v", "./"+filepath.ToSlash(filepath.Join("addons", addonName))+":/opt/addons/"+addonName,
		)
		addonPaths = append(addonPaths, "/opt/addons/"+addonName)
	}

	containerArgs = append(containerArgs, image, "odoo", "--dev=all")
	if len(addonNames) > 0 {
		containerArgs = append(containerArgs, "--addons-path="+strings.Join(addonPaths, ","))
	}
	containerArgs = append(containerArgs, databaseFilter(prefix))
	fmt.Printf("starting profile %q\n", name)
	if err := dockerQuiet(containerArgs...); err != nil {
		return fmt.Errorf("create Odoo container: %w", err)
	}

	previousState := cloneState(state)
	if err := files.AddContainer(state, name); err != nil {
		cleanupErr := removeCreatedContainer(containerName)
		restoreState(state, previousState)
		return combineErrors(fmt.Errorf("update workspace: %w", err), cleanupErr)
	}
	if err := files.SetContainerVersion(state, name, selectedVersion); err != nil {
		cleanupErr := removeCreatedContainer(containerName)
		restoreState(state, previousState)
		return combineErrors(fmt.Errorf("update workspace: %w", err), cleanupErr)
	}
	if err := proxy.Sync(state); err != nil {
		cleanupErr := removeCreatedContainer(containerName)
		restoreState(state, previousState)
		return combineErrors(fmt.Errorf("synchronize Caddy routing: %w", err), cleanupErr)
	}
	return reportContainerURL(name)
}

func databaseFilter(prefix string) string {
	return "--db-filter=^" + regexp.QuoteMeta(prefix) + ".*$"
}

func reportContainerURL(name string) error {
	hostname := profileHostname(name)
	fmt.Printf("profile running at http://%s\n", hostname)
	return nil
}

func removeCreatedContainer(name string) error {
	if err := dockerQuiet("rm", "-f", name); err != nil {
		return fmt.Errorf("remove created container %q: %w", name, err)
	}
	return nil
}

func cloneState(state files.State) files.State {
	clone := make(files.State, len(state))
	for key, value := range state {
		clone[key] = append([]byte(nil), value...)
	}
	return clone
}

func restoreState(state, snapshot files.State) {
	for key := range state {
		delete(state, key)
	}
	for key, value := range snapshot {
		state[key] = append([]byte(nil), value...)
	}
}

func combineErrors(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	return fmt.Errorf("%w; cleanup failed: %v", primary, cleanup)
}

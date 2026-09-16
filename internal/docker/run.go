package docker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"lidoo/internal/addons"
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
	return buildImageWithOptions(dockerfile, image, CommandOptions{})
}

func buildImageWithOptions(dockerfile, image string, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	buildx := dockerCommandAvailableWithContext(options.Context, "buildx", "version")
	return dockerQuietWithOptions(options, buildImageArgs(dockerfile, image, buildx)...)
}

func Run(name, version string, state files.State) error {
	return RunWithOptions(name, version, state, CommandOptions{})
}

func RunWithOptions(name, version string, state files.State, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	if name == "" {
		return errors.New("run requires name")
	}
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	storedVersion, err := profile.Version(state, name)
	if err != nil {
		return err
	}
	selectedVersion := version
	if storedVersion != nil {
		selectedVersion = *storedVersion
		if version != "" {
			fmt.Fprintf(options.Stderr, "warning: container %q uses version %q; ignoring --version %q\n", name, selectedVersion, version)
		}
	} else if version == "" {
		return fmt.Errorf("run requires --version because container %q has no stored version", name)
	}
	if !odooVersion.MatchString(selectedVersion) {
		return fmt.Errorf("invalid Odoo version %q", selectedVersion)
	}

	addonNames, err := profile.Addons(state, name)
	if err != nil {
		return err
	}
	mounts, err := addons.ResolveMounts(state, addonNames)
	if err != nil {
		return fmt.Errorf("validate profile addons: %w", err)
	}
	prefix, err := profile.Prefix(state, name)
	if err != nil {
		return err
	}

	containerName := profile.ContainerName(name)
	exists, err := findContainerByNameWithOptions(name, options)
	if err != nil {
		return err
	}
	if exists {
		previousState := files.CloneState(state)
		running, err := containerIsRunningWithOptions(name, options)
		if err != nil {
			return err
		}
		if running {
			if err := ensureProfileState(state, name, options.Stdout); err != nil {
				files.RestoreState(state, previousState)
				return fmt.Errorf("update state: %w", err)
			}
			if err := proxy.SyncWithContext(options.Context, state); err != nil {
				files.RestoreState(state, previousState)
				return fmt.Errorf("synchronize Caddy routing: %w", err)
			}
			return fmt.Errorf("container %q already exists", containerName)
		}
		if err := dockerQuietWithOptions(options, "start", containerName); err != nil {
			return fmt.Errorf("start container %q: %w", containerName, err)
		}
		if err := ensureProfileState(state, name, options.Stdout); err != nil {
			_ = dockerQuietWithOptions(options, "stop", containerName)
			files.RestoreState(state, previousState)
			return fmt.Errorf("update state: %w", err)
		}
		if err := proxy.SyncWithContext(options.Context, state); err != nil {
			_ = dockerQuietWithOptions(options, "stop", containerName)
			files.RestoreState(state, previousState)
			return fmt.Errorf("synchronize Caddy routing: %w", err)
		}
		return reportContainerURL(name, options.Stdout)
	}

	dockerfile := filepath.Join("docker", "Dockerfile."+selectedVersion)
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("Dockerfile for Odoo %s not found: %s", selectedVersion, dockerfile)
	}

	if _, err := os.Stat(databaseEnvFile); err != nil {
		return fmt.Errorf("%q not found: %w", databaseEnvFile, err)
	}
	if err := networkExistsWithContext(options.Context); err != nil {
		return err
	}
	if err := EnsureVolumeWithContext(options.Context); err != nil {
		return fmt.Errorf("ensure filestore volume: %w", err)
	}

	image := "lidoo-odoo:" + selectedVersion
	fmt.Fprintf(options.Stdout, "building Odoo %s image\n", selectedVersion)
	if err := buildImageWithOptions(dockerfile, image, options); err != nil {
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
	for _, mount := range mounts {
		containerArgs = append(containerArgs,
			"-v", filepath.ToSlash(mount.Path)+":/opt/addons/"+mount.Name,
		)
		addonPaths = append(addonPaths, "/opt/addons/"+mount.Name)
	}

	containerArgs = append(containerArgs, image, "odoo", "--dev=all")
	if len(addonNames) > 0 {
		containerArgs = append(containerArgs, "--addons-path="+strings.Join(addonPaths, ","))
	}
	containerArgs = append(containerArgs, databaseFilter(prefix))
	fmt.Fprintf(options.Stdout, "starting profile %q\n", name)
	if err := dockerQuietWithOptions(options, containerArgs...); err != nil {
		return fmt.Errorf("create Odoo container: %w", err)
	}

	previousState := files.CloneState(state)
	if err := ensureProfileState(state, name, options.Stdout); err != nil {
		cleanupErr := removeCreatedContainer(containerName, options)
		files.RestoreState(state, previousState)
		return combineErrors(fmt.Errorf("update workspace: %w", err), cleanupErr)
	}
	if err := profile.SetVersion(state, name, selectedVersion); err != nil {
		cleanupErr := removeCreatedContainer(containerName, options)
		files.RestoreState(state, previousState)
		return combineErrors(fmt.Errorf("update workspace: %w", err), cleanupErr)
	}
	if err := proxy.SyncWithContext(options.Context, state); err != nil {
		cleanupErr := removeCreatedContainer(containerName, options)
		files.RestoreState(state, previousState)
		return combineErrors(fmt.Errorf("synchronize Caddy routing: %w", err), cleanupErr)
	}
	return reportContainerURL(name, options.Stdout)
}

func databaseFilter(prefix string) string {
	return "--db-filter=^" + regexp.QuoteMeta(prefix) + ".*$"
}

func reportContainerURL(name string, output io.Writer) error {
	hostname := profileHostname(name)
	fmt.Fprintf(output, "profile running at http://%s\n", hostname)
	return nil
}

func removeCreatedContainer(name string, options CommandOptions) error {
	if err := dockerQuietWithOptions(options, "rm", "-f", name); err != nil {
		return fmt.Errorf("remove created container %q: %w", name, err)
	}
	return nil
}

func ensureProfileState(state files.State, name string, output io.Writer) error {
	created, err := profile.Ensure(state, name)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(output, "\033[32mstate entry for container %q created\033[0m\n", name)
	}
	return nil
}

func combineErrors(primary, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	return fmt.Errorf("%w; cleanup failed: %v", primary, cleanup)
}

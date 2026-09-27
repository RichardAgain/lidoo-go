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

func ValidateProfileVersion(version string) error {
	if !odooVersion.MatchString(version) {
		return fmt.Errorf("invalid Odoo version %q", version)
	}
	return nil
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
	if err := networkExistsWithContext(options.Context); err != nil {
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
	if err := ValidateProfileVersion(selectedVersion); err != nil {
		return err
	}

	config, found, err := profile.Lookup(state, name)
	if err != nil {
		return err
	}
	if !found {
		config = profile.NewConfig(name)
	}
	mounts, err := addons.ResolveMounts(state, config.Addons)
	if err != nil {
		return fmt.Errorf("validate profile addons: %w", err)
	}
	filterArgs, err := databaseFilterArgs(config)
	if err != nil {
		return err
	}
	addonNames := config.Addons
	runtimeConfig, err := writeRuntimeConfig(name, config.AdminPasswd)
	if err != nil {
		return fmt.Errorf("write runtime Odoo config: %w", err)
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
	if runtimeConfig != "" {
		containerArgs = append(containerArgs,
			"--mount", "type=bind,source="+runtimeConfig+",target=/etc/odoo/odoo.conf,readonly",
		)
	}

	addonPaths := []string{"/usr/lib/python3/dist-packages/odoo/addons"}
	for _, mount := range mounts {
		containerArgs = append(containerArgs,
			"-v", filepath.ToSlash(mount.Path)+":/opt/addons/"+mount.Name,
		)
		addonPaths = append(addonPaths, "/opt/addons/"+mount.Name)
	}

	addonPath := strings.Join(addonPaths, ",")
	if len(addonNames) > 0 {
		containerArgs = append(containerArgs, "--env", "LIDOO_ADDONS_PATH="+addonPath)
	}
	containerArgs = append(containerArgs, image, "odoo", "--dev=all")
	if len(addonNames) > 0 {
		containerArgs = append(containerArgs, "--addons-path="+addonPath)
	}
	containerArgs = append(containerArgs, filterArgs...)
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

func databaseFilterArgs(config profile.Config) ([]string, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	switch config.EffectiveDBFilterMode() {
	case profile.DBFilterModeProfile:
		return []string{databaseFilter(config.Prefix)}, nil
	case profile.DBFilterModeDisabled:
		return nil, nil
	case profile.DBFilterModeCustom:
		return []string{"--db-filter=" + config.DBFilterPattern}, nil
	default:
		return nil, fmt.Errorf("invalid database filter mode %q", config.DBFilterMode)
	}
}

func runtimeConfigPath(name string) string {
	return filepath.Join(".lidoo", name, "odoo.conf")
}

func writeRuntimeConfig(name, adminPasswd string) (string, error) {
	path := runtimeConfigPath(name)
	if adminPasswd == "" {
		// Do not create a config for an unset password. If a previous config is
		// mounted by a stopped container, rewrite it without the old secret so
		// that the existing mount still contains a valid Odoo config.
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return "", nil
			}
			return "", err
		}
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		return "", err
	}
	contents := "[options]\naddons_path = /mnt/extra-addons\ndata_dir = /var/lib/odoo\n"
	if adminPasswd != "" {
		contents += "admin_passwd = " + adminPasswd + "\n"
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return "", err
	}
	if adminPasswd == "" {
		return "", nil
	}
	return path, nil
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

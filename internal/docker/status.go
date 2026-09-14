package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"lidoo/internal/addons"
	"lidoo/internal/files"
	"lidoo/internal/profile"
)

type runtimeProfile struct {
	ID      string
	State   string
	Image   string
	Version string
	Mounts  []runtimeMount
}

type runtimeMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

type inspectedContainer struct {
	ID    string `json:"Id"`
	State struct {
		Status string `json:"Status"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	Mounts []runtimeMount `json:"Mounts"`
}

func Status(name string, state files.State) error {
	runtime, err := runtimeProfiles()
	if err != nil {
		return fmt.Errorf("inspect profile containers: %w", err)
	}

	names, err := profile.Names(state)
	if err != nil {
		return err
	}
	if name != "" {
		if err := profile.ValidateName(name); err != nil {
			return err
		}
		names = []string{name}
		if _, found, err := profile.Lookup(state, name); err != nil {
			return err
		} else if !found {
			if _, running := runtime[name]; !running {
				return fmt.Errorf("profile %q not found", name)
			}
		}
	} else {
		seen := make(map[string]bool, len(names)+len(runtime))
		for _, profileName := range names {
			seen[profileName] = true
		}
		for profileName := range runtime {
			if !seen[profileName] {
				names = append(names, profileName)
			}
		}
		sort.Strings(names)
	}
	if len(names) == 0 {
		fmt.Println("no profiles found")
		return nil
	}

	for index, profileName := range names {
		if index > 0 {
			fmt.Println()
		}
		if err := renderStatus(profileName, state, runtime[profileName]); err != nil {
			return err
		}
	}
	return nil
}

func runtimeProfiles() (map[string]runtimeProfile, error) {
	output, err := dockerOutput(
		"ps", "--all",
		"--filter", "label="+containerNameLabel,
		"--format", `{{.Label "io.lidoo.name"}}\t{{.ID}}`,
	)
	if err != nil {
		return nil, err
	}
	profiles := make(map[string]runtimeProfile)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
			return nil, fmt.Errorf("invalid profile row from Docker: %q", line)
		}
		name := strings.TrimSpace(fields[0])
		if err := profile.ValidateName(name); err != nil {
			return nil, fmt.Errorf("invalid profile from Docker: %w", err)
		}
		inspected, err := inspectContainer(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("inspect profile %q: %w", name, err)
		}
		profiles[name] = runtimeProfile{
			ID:      inspected.ID,
			State:   inspected.State.Status,
			Image:   inspected.Config.Image,
			Version: inspected.Config.Labels["io.lidoo.odoo-version"],
			Mounts:  inspected.Mounts,
		}
	}
	return profiles, nil
}

func inspectContainer(id string) (inspectedContainer, error) {
	output, err := dockerOutput("inspect", "--format", "{{json .}}", id)
	if err != nil {
		return inspectedContainer{}, err
	}
	var inspected inspectedContainer
	if err := json.Unmarshal(output, &inspected); err != nil {
		return inspectedContainer{}, err
	}
	return inspected, nil
}

func renderStatus(name string, state files.State, runtime runtimeProfile) error {
	config, found, err := profile.Lookup(state, name)
	if err != nil {
		return fmt.Errorf("read profile %q: %w", name, err)
	}
	if !found {
		config = profile.NewConfig(name)
	}

	version := "-"
	if config.Version != nil {
		version = *config.Version
	}
	if runtime.Version != "" {
		version = runtime.Version
	}
	image := "-"
	dockerState := "not created"
	filestore := FilestoreVolumeName + " (not attached)"
	pending := "no (not created)"
	if runtime.ID != "" {
		dockerState = runtime.State
		image = runtime.Image
		filestore = mountDisplay(runtime.Mounts, "/var/lib/odoo")
		pending = recreationStatus(state, config, runtime.Mounts)
	}

	fmt.Printf("PROFILE: %s\n", name)
	fmt.Printf("docker state: %s\n", dockerState)
	fmt.Printf("url: http://%s\n", profile.Hostname(name))
	fmt.Printf("odoo version: %s\n", version)
	fmt.Printf("attached addons: %s\n", strings.Join(config.Addons, ", "))
	fmt.Printf("database prefix: %s\n", config.Prefix)
	fmt.Printf("image: %s\n", image)
	fmt.Printf("filestore volume: %s\n", filestore)
	fmt.Printf("pending recreation: %s\n", pending)
	return nil
}

func mountDisplay(mounts []runtimeMount, destination string) string {
	for _, mount := range mounts {
		if mount.Destination != destination {
			continue
		}
		if mount.Name != "" {
			return mount.Name
		}
		if mount.Source != "" {
			return mount.Source
		}
	}
	return "not attached"
}

func recreationStatus(state files.State, config profile.Config, mounts []runtimeMount) string {
	expected, err := addons.ResolveMounts(state, config.Addons)
	if err != nil {
		return "yes (invalid addon mounts: " + err.Error() + ")"
	}
	actual := make(map[string]string)
	for _, mount := range mounts {
		if strings.HasPrefix(mount.Destination, "/opt/addons/") {
			actual[mount.Destination] = filepath.Clean(mount.Source)
		}
	}
	for _, mount := range expected {
		destination := "/opt/addons/" + mount.Name
		if filepath.Clean(actual[destination]) != filepath.Clean(mount.Path) {
			return "yes"
		}
		delete(actual, destination)
	}
	if len(actual) > 0 {
		return "yes"
	}
	filestoreFound := false
	for _, mount := range mounts {
		if mount.Destination != "/var/lib/odoo" {
			continue
		}
		filestoreFound = true
		if mount.Name != FilestoreVolumeName {
			return "yes"
		}
	}
	if !filestoreFound {
		return "yes"
	}
	return "no"
}

func Logs(name string, follow bool, tail int) error {
	if name == "" {
		return fmt.Errorf("logs requires name")
	}
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	containers, err := containerIDs("label="+containerNameLabel+"="+name, true)
	if err != nil {
		return fmt.Errorf("find profile %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no profile with name %q", name)
	}
	args := []string{"logs"}
	if follow {
		args = append(args, "--follow")
	}
	if tail >= 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	args = append(args, containers[0])
	return runDockerCommand(args...)
}

func runDockerCommand(args ...string) error {
	command := exec.Command("docker", args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

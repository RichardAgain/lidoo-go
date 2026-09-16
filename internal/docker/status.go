package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"lidoo/internal/addons"
	"lidoo/internal/files"
	"lidoo/internal/profile"
)

// ProfileDetail is the inspection data for one profile.
type ProfileDetail struct {
	Name              string
	DockerState       string
	URL               string
	OdooVersion       string
	AttachedAddons    []string
	DatabasePrefix    string
	Image             string
	FilestoreVolume   string
	PendingRecreation RecreationStatus
}

// RecreationStatus describes whether a running profile must be recreated for
// its configured mounts to match the current workspace state.
type RecreationStatus struct {
	Checked bool
	Pending bool
	Reason  string
}

func (status RecreationStatus) String() string {
	if !status.Checked {
		return "no (not created)"
	}
	if !status.Pending {
		return "no"
	}
	if status.Reason != "" {
		return "yes (" + status.Reason + ")"
	}
	return "yes"
}

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

// ProfileDetails returns inspection data for profiles known by workspace state,
// Docker, or both. When name is non-empty, only that profile is returned.
func ProfileDetails(name string, state files.State) ([]ProfileDetail, error) {
	runtime, err := runtimeProfiles()
	if err != nil {
		return nil, fmt.Errorf("inspect profile containers: %w", err)
	}

	names, err := profile.Names(state)
	if err != nil {
		return nil, err
	}
	if name != "" {
		if err := profile.ValidateName(name); err != nil {
			return nil, err
		}
		if _, found, err := profile.Lookup(state, name); err != nil {
			return nil, err
		} else if !found {
			if _, exists := runtime[name]; !exists {
				return nil, fmt.Errorf("profile %q not found", name)
			}
		}
		names = []string{name}
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

	details := make([]ProfileDetail, 0, len(names))
	for _, profileName := range names {
		detail, err := profileDetail(profileName, state, runtime[profileName])
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func profileDetail(name string, state files.State, runtime runtimeProfile) (ProfileDetail, error) {
	config, found, err := profile.Lookup(state, name)
	if err != nil {
		return ProfileDetail{}, fmt.Errorf("read profile %q: %w", name, err)
	}
	if !found {
		config = profile.NewConfig(name)
	}

	detail := ProfileDetail{
		Name:              name,
		DockerState:       "not created",
		URL:               "http://" + profile.Hostname(name),
		OdooVersion:       profileVersion(config),
		AttachedAddons:    append([]string(nil), config.Addons...),
		DatabasePrefix:    config.Prefix,
		Image:             "-",
		FilestoreVolume:   FilestoreVolumeName + " (not attached)",
		PendingRecreation: RecreationStatus{},
	}
	if runtime.ID == "" {
		return detail, nil
	}

	detail.DockerState = runtime.State
	detail.Image = runtime.Image
	detail.FilestoreVolume = mountDisplay(runtime.Mounts, "/var/lib/odoo")
	if runtime.Version != "" {
		detail.OdooVersion = runtime.Version
	}
	detail.PendingRecreation = recreationStatus(state, config, runtime.Mounts)
	return detail, nil
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

func recreationStatus(state files.State, config profile.Config, mounts []runtimeMount) RecreationStatus {
	expected, err := addons.ResolveMounts(state, config.Addons)
	if err != nil {
		return RecreationStatus{
			Checked: true,
			Pending: true,
			Reason:  "invalid addon mounts: " + err.Error(),
		}
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
			return RecreationStatus{Checked: true, Pending: true}
		}
		delete(actual, destination)
	}
	if len(actual) > 0 {
		return RecreationStatus{Checked: true, Pending: true}
	}
	filestoreFound := false
	for _, mount := range mounts {
		if mount.Destination != "/var/lib/odoo" {
			continue
		}
		filestoreFound = true
		if mount.Name != FilestoreVolumeName {
			return RecreationStatus{Checked: true, Pending: true}
		}
	}
	if !filestoreFound {
		return RecreationStatus{Checked: true, Pending: true}
	}
	return RecreationStatus{Checked: true}
}

func Logs(name string, follow bool, tail int) error {
	command, err := LogsCommand(name, follow, tail)
	if err != nil {
		return err
	}
	return runDockerCommand(command.Args[1:]...)
}

func LogsCommand(name string, follow bool, tail int) (*exec.Cmd, error) {
	return logsCommand(context.Background(), name, follow, tail, os.Stderr)
}

func LogsCommandWithContext(ctx context.Context, name string, follow bool, tail int) (*exec.Cmd, error) {
	return logsCommand(ctx, name, follow, tail, io.Discard)
}

func logsCommand(ctx context.Context, name string, follow bool, tail int, lookupStderr io.Writer) (*exec.Cmd, error) {
	if name == "" {
		return nil, fmt.Errorf("logs requires name")
	}
	if err := profile.ValidateName(name); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, true, CommandOptions{Context: ctx, Stderr: lookupStderr})
	if err != nil {
		return nil, fmt.Errorf("find profile %q: %w", name, err)
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("no profile with name %q", name)
	}
	args := []string{"logs"}
	if follow {
		args = append(args, "--follow")
	}
	if tail >= 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	args = append(args, containers[0])
	return exec.CommandContext(ctx, "docker", args...), nil
}

func runDockerCommand(args ...string) error {
	command := exec.Command("docker", args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

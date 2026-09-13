package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const statePath = ".lidoo.json"

type State map[string]json.RawMessage

func ReadState() (State, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	state := make(State)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = make(State)
	}
	return state, nil
}

func SaveState(state State) error {
	output, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(output, '\n'), 0o644)
}

type AddonEntry struct {
	Path       string `json:"path"`
	Source     string `json:"source,omitempty"`
	WorktreeOf string `json:"worktreeOf,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

type containerEntry struct {
	Addons  []string `json:"addons"`
	Prefix  string   `json:"prefix"`
	Version *string  `json:"version,omitempty"`
}

func loadAddons(state State) (map[string]AddonEntry, error) {
	addons := make(map[string]AddonEntry)
	if raw, ok := state["addons"]; ok {
		if err := json.Unmarshal(raw, &addons); err != nil {
			return nil, fmt.Errorf("read addons: %w", err)
		}
		if addons == nil {
			addons = make(map[string]AddonEntry)
		}
	}
	return addons, nil
}

func saveAddons(state State, addons map[string]AddonEntry) error {
	rawAddons, err := json.Marshal(addons)
	if err != nil {
		return err
	}
	state["addons"] = rawAddons
	return nil
}

func LookupAddon(state State, name string) (AddonEntry, bool, error) {
	addons, err := loadAddons(state)
	if err != nil {
		return AddonEntry{}, false, err
	}
	addon, ok := addons[name]
	return addon, ok, nil
}

func RegisterAddon(state State, name, source, path string) error {
	addons, err := loadAddons(state)
	if err != nil {
		return err
	}
	addons[name] = AddonEntry{Path: filepath.ToSlash(path), Source: source}
	return saveAddons(state, addons)
}

func RegisterWorktree(state State, name, path, parent, branch string) error {
	addons, err := loadAddons(state)
	if err != nil {
		return err
	}
	addons[name] = AddonEntry{
		Path:       filepath.ToSlash(path),
		WorktreeOf: parent,
		Branch:     branch,
	}
	return saveAddons(state, addons)
}

func UnregisterAddon(state State, name string) error {
	addons, err := loadAddons(state)
	if err != nil {
		return err
	}
	if _, ok := addons[name]; !ok {
		return fmt.Errorf("addon %q not found", name)
	}
	delete(addons, name)
	return saveAddons(state, addons)
}

func AddonProfiles(state State, name string) ([]string, error) {
	if raw, ok := state["containers"]; ok {
		containers := make(map[string]containerEntry)
		if err := json.Unmarshal(raw, &containers); err != nil {
			return nil, fmt.Errorf("read containers: %w", err)
		}

		profiles := make([]string, 0)
		for container, config := range containers {
			for _, addon := range config.Addons {
				if addon == name {
					profiles = append(profiles, container)
					break
				}
			}
		}
		sort.Strings(profiles)
		return profiles, nil
	}
	return nil, nil
}

func ChildWorktrees(state State, parent string) ([]string, error) {
	addons, err := loadAddons(state)
	if err != nil {
		return nil, err
	}

	children := make([]string, 0)
	for name, addon := range addons {
		if addon.WorktreeOf == parent {
			children = append(children, name)
		}
	}
	sort.Strings(children)
	return children, nil
}

func ContainerNames(state State) ([]string, error) {
	containers := make(map[string]containerEntry)
	if raw, ok := state["containers"]; ok {
		if err := json.Unmarshal(raw, &containers); err != nil {
			return nil, fmt.Errorf("read containers: %w", err)
		}
	}

	names := make([]string, 0, len(containers))
	for name := range containers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func RemoveContainer(state State, container string) error {
	containers := make(map[string]containerEntry)
	raw, ok := state["containers"]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(raw, &containers); err != nil {
		return fmt.Errorf("read containers: %w", err)
	}
	if _, ok := containers[container]; !ok {
		return nil
	}
	delete(containers, container)

	rawContainers, err := json.Marshal(containers)
	if err != nil {
		return err
	}
	state["containers"] = rawContainers
	return nil
}

func AddContainer(state State, container string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}

	containers := make(map[string]containerEntry)
	if raw, ok := state["containers"]; ok {
		if err := json.Unmarshal(raw, &containers); err != nil {
			return fmt.Errorf("read containers: %w", err)
		}
		if containers == nil {
			containers = make(map[string]containerEntry)
		}
	}
	if _, ok := containers[container]; ok {
		return nil
	}
	containers[container] = containerEntry{
		Addons: []string{},
		Prefix: container + "__",
	}
	rawContainers, err := json.Marshal(containers)
	if err != nil {
		return err
	}
	state["containers"] = rawContainers
	fmt.Printf("\033[32mstate entry for container %q created\033[0m\n", container)
	return nil
}

func SetContainerVersion(state State, container, version string) error {
	if strings.TrimSpace(version) == "" {
		return errors.New("container version cannot be empty")
	}

	containers := make(map[string]containerEntry)
	if raw, ok := state["containers"]; ok {
		if err := json.Unmarshal(raw, &containers); err != nil {
			return fmt.Errorf("read containers: %w", err)
		}
	}
	config, ok := containers[container]
	if !ok {
		return fmt.Errorf("container %q not found", container)
	}
	config.Version = &version
	containers[container] = config

	rawContainers, err := json.Marshal(containers)
	if err != nil {
		return err
	}
	state["containers"] = rawContainers
	return nil
}

func AddAddonsToContainer(state State, container string, names []string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}
	if len(names) == 0 {
		return errors.New("at least one addon is required")
	}

	containers := make(map[string]containerEntry)
	if raw, ok := state["containers"]; ok {
		if err := json.Unmarshal(raw, &containers); err != nil {
			return fmt.Errorf("read containers: %w", err)
		}
		if containers == nil {
			containers = make(map[string]containerEntry)
		}
	}

	config, ok := containers[container]
	if !ok {
		config = containerEntry{Prefix: container + "__"}
	}
	seen := make(map[string]bool, len(config.Addons))
	for _, name := range config.Addons {
		seen[name] = true
	}
	for _, name := range names {
		if seen[name] {
			continue
		}
		config.Addons = append(config.Addons, name)
		seen[name] = true
	}
	containers[container] = config

	rawContainers, err := json.Marshal(containers)
	if err != nil {
		return err
	}
	state["containers"] = rawContainers
	return nil
}

func RemoveAddonsFromContainer(state State, container string, names []string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}
	if len(names) == 0 {
		return errors.New("at least one addon is required")
	}

	containers := make(map[string]containerEntry)
	raw, ok := state["containers"]
	if !ok {
		return fmt.Errorf("container %q not found", container)
	}
	if err := json.Unmarshal(raw, &containers); err != nil {
		return fmt.Errorf("read containers: %w", err)
	}
	config, ok := containers[container]
	if !ok {
		return fmt.Errorf("container %q not found", container)
	}

	remove := make(map[string]bool, len(names))
	for _, name := range names {
		remove[name] = true
	}
	remaining := make([]string, 0, len(config.Addons))
	for _, addon := range config.Addons {
		if !remove[addon] {
			remaining = append(remaining, addon)
		}
	}
	config.Addons = remaining
	containers[container] = config

	rawContainers, err := json.Marshal(containers)
	if err != nil {
		return err
	}
	state["containers"] = rawContainers
	return nil
}

func ContainerAddons(state State, container string) ([]string, error) {
	if raw, ok := state["containers"]; ok {
		containers := make(map[string]containerEntry)
		if err := json.Unmarshal(raw, &containers); err != nil {
			return nil, fmt.Errorf("read containers: %w", err)
		}
		return containers[container].Addons, nil
	}
	return nil, nil
}

func ContainerPrefix(state State, container string) (string, error) {
	if raw, ok := state["containers"]; ok {
		containers := make(map[string]containerEntry)
		if err := json.Unmarshal(raw, &containers); err != nil {
			return "", fmt.Errorf("read containers: %w", err)
		}
		if config, ok := containers[container]; ok {
			return config.Prefix, nil
		}
	}
	return container + "__", nil
}

func ContainerVersion(state State, container string) (*string, error) {
	if raw, ok := state["containers"]; ok {
		containers := make(map[string]containerEntry)
		if err := json.Unmarshal(raw, &containers); err != nil {
			return nil, fmt.Errorf("read containers: %w", err)
		}
		if config, ok := containers[container]; ok {
			return config.Version, nil
		}
	}
	return nil, nil
}

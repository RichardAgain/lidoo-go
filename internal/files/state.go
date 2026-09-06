package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

type addonEntry struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

type containerEntry struct {
	Addons []string `json:"addons"`
	Prefix string   `json:"prefix"`
}

func RegisterAddon(state State, name, source, path string) error {
	addons := make(map[string]addonEntry)
	if raw, ok := state["addons"]; ok {
		if err := json.Unmarshal(raw, &addons); err != nil {
			return fmt.Errorf("read addons: %w", err)
		}
		if addons == nil {
			addons = make(map[string]addonEntry)
		}
	}
	addons[name] = addonEntry{Path: filepath.ToSlash(path), Source: source}

	rawAddons, err := json.Marshal(addons)
	if err != nil {
		return err
	}
	state["addons"] = rawAddons
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

package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const workspacePath = ".workspace.json"

type Workspace map[string]json.RawMessage

func ReadWorkspace() (Workspace, error) {
	data, err := os.ReadFile(workspacePath)
	if err != nil {
		return nil, err
	}

	workspace := make(Workspace)
	if err := json.Unmarshal(data, &workspace); err != nil {
		return nil, err
	}
	if workspace == nil {
		workspace = make(Workspace)
	}
	return workspace, nil
}

func SaveWorkspace(workspace Workspace) error {
	output, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(workspacePath, append(output, '\n'), 0o644)
}

type workspaceAddon struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

type workspaceContainer struct {
	Addons []string `json:"addons"`
}

func UpdateWorkspace(workspace Workspace, name, source, path string) error {
	addons := make(map[string]workspaceAddon)
	if raw, ok := workspace["addons"]; ok {
		if err := json.Unmarshal(raw, &addons); err != nil {
			return fmt.Errorf("read addons: %w", err)
		}
		if addons == nil {
			addons = make(map[string]workspaceAddon)
		}
	}
	addons[name] = workspaceAddon{Path: filepath.ToSlash(path), Source: source}

	rawAddons, err := json.Marshal(addons)
	if err != nil {
		return err
	}
	workspace["addons"] = rawAddons
	return nil
}

func AddContainer(workspace Workspace, container string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}

	containers := make(map[string]workspaceContainer)
	if raw, ok := workspace["containers"]; ok {
		if err := json.Unmarshal(raw, &containers); err != nil {
			return fmt.Errorf("read containers: %w", err)
		}
		if containers == nil {
			containers = make(map[string]workspaceContainer)
		}
	}
	if _, ok := containers[container]; ok {
		return nil
	}

	containers[container] = workspaceContainer{Addons: []string{}}
	rawContainers, err := json.Marshal(containers)
	if err != nil {
		return err
	}
	workspace["containers"] = rawContainers
	fmt.Printf("\033[32mworkspace entry for container %q created\033[0m\n", container)
	return nil
}

func AddAddonsToContainer(workspace Workspace, container string, names []string) error {
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}
	if len(names) == 0 {
		return errors.New("at least one addon is required")
	}

	containers := make(map[string]workspaceContainer)
	if raw, ok := workspace["containers"]; ok {
		if err := json.Unmarshal(raw, &containers); err != nil {
			return fmt.Errorf("read containers: %w", err)
		}
		if containers == nil {
			containers = make(map[string]workspaceContainer)
		}
	}

	config := containers[container]
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
	workspace["containers"] = rawContainers
	return nil
}

func ContainerAddons(workspace Workspace, container string) ([]string, error) {
	if raw, ok := workspace["containers"]; ok {
		containers := make(map[string]workspaceContainer)
		if err := json.Unmarshal(raw, &containers); err != nil {
			return nil, fmt.Errorf("read containers: %w", err)
		}
		return containers[container].Addons, nil
	}
	return nil, nil
}

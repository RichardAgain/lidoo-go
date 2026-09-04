package files

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

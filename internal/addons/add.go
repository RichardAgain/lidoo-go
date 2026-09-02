package addons

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	addonsDir     = "addons"
	workspaceFile = ".workspace.json"
)

func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("addons requires a subcommand")
	}

	switch args[0] {
	case "add":
		return Add(args[1:])
	default:
		return fmt.Errorf("unknown addons command %q", args[0])
	}
}

func Add(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: lidoo addons add <addon name> <git url>")
	}

	name, url := args[0], args[1]
	if !validAddonName(name) {
		return fmt.Errorf("invalid addon name %q", name)
	}
	if strings.TrimSpace(url) == "" {
		return errors.New("git url cannot be empty")
	}

	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return fmt.Errorf("create addons directory: %w", err)
	}

	destination := filepath.Join(addonsDir, name)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("addon %q already exists", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check addon %q: %w", name, err)
	}

	cmd := exec.Command("git", "clone", "--", url, destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone addon %q: %w", name, err)
	}
	if err := updateWorkspace(name, url, destination); err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	return nil
}

type workspaceAddon struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

func updateWorkspace(name, source, path string) error {
	data, err := os.ReadFile(workspaceFile)
	if err != nil {
		return err
	}

	workspace := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &workspace); err != nil {
		return err
	}
	if workspace == nil {
		workspace = make(map[string]json.RawMessage)
	}

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

	output, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(workspaceFile, append(output, '\n'), 0o644)
}

func validAddonName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		strings.TrimSpace(name) == name &&
		!strings.ContainsAny(name, `/\\`) &&
		!strings.ContainsRune(name, 0)
}

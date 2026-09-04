package addons

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"lidoo/internal/files"
)

const addonsDir = "addons"

func Run(args []string, workspace files.Workspace) error {
	if len(args) == 0 {
		return errors.New("addons requires a subcommand")
	}

	switch args[0] {
	case "add":
		return Add(args[1:], workspace)
	default:
		return fmt.Errorf("unknown addons command %q", args[0])
	}
}

func Add(args []string, workspace files.Workspace) error {
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
	if err := files.UpdateWorkspace(workspace, name, url, destination); err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	return nil
}

func validAddonName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		strings.TrimSpace(name) == name &&
		!strings.ContainsAny(name, `/\\`) &&
		!strings.ContainsRune(name, 0)
}

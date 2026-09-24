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

type AddOptions struct {
	Depth  int
	Branch string
}

func Add(name, url string, options AddOptions, state files.State) error {
	operationOptions := OperationOptions{}
	return AddWithOptions(name, url, options, state, operationOptions)
}

func AddWithOptions(name, url string, cloneOptions AddOptions, state files.State, operationOptions OperationOptions) error {
	operationOptions = normalizeOperationOptions(operationOptions)
	if !validAddonName(name) {
		return fmt.Errorf("invalid addon name %q", name)
	}
	if strings.TrimSpace(url) == "" {
		return errors.New("git url cannot be empty")
	}
	if cloneOptions.Depth < 0 {
		return errors.New("clone depth cannot be negative")
	}
	if cloneOptions.Branch != "" && (strings.TrimSpace(cloneOptions.Branch) != cloneOptions.Branch || strings.HasPrefix(cloneOptions.Branch, "-")) {
		return fmt.Errorf("invalid branch %q", cloneOptions.Branch)
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

	args := []string{"clone"}
	if cloneOptions.Depth > 0 {
		args = append(args, "--depth", fmt.Sprint(cloneOptions.Depth))
	}
	if cloneOptions.Branch != "" {
		args = append(args, "--branch", cloneOptions.Branch)
	}
	args = append(args, "--", url, destination)
	cmd := exec.CommandContext(operationOptions.Context, "git", args...)
	cmd.Stdout = operationOptions.Output
	cmd.Stderr = operationOptions.ErrorOutput
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone addon %q: %w", name, err)
	}
	if err := registerAddon(state, name, url, destination); err != nil {
		return fmt.Errorf("register addon: %w", err)
	}
	return nil
}

func validAddonName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		strings.TrimSpace(name) == name &&
		!strings.ContainsAny(name, `/\\`) &&
		!strings.ContainsRune(name, 0)
}

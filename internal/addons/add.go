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
	if !validAddonName(name) {
		return fmt.Errorf("invalid addon name %q", name)
	}
	if strings.TrimSpace(url) == "" {
		return errors.New("git url cannot be empty")
	}
	if options.Depth < 0 {
		return errors.New("clone depth cannot be negative")
	}
	if options.Branch != "" && (strings.TrimSpace(options.Branch) != options.Branch || strings.HasPrefix(options.Branch, "-")) {
		return fmt.Errorf("invalid branch %q", options.Branch)
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
	if options.Depth > 0 {
		args = append(args, "--depth", fmt.Sprint(options.Depth))
	}
	if options.Branch != "" {
		args = append(args, "--branch", options.Branch)
	}
	args = append(args, "--", url, destination)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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

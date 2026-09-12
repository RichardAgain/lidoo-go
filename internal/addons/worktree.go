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

func Worktree(source, name, branch string, state files.State) error {
	if !validAddonName(name) {
		return fmt.Errorf("invalid addon name %q", name)
	}
	if strings.TrimSpace(branch) == "" {
		return errors.New("branch cannot be empty")
	}

	sourceAddon, found, err := files.LookupAddon(state, source)
	if err != nil {
		return fmt.Errorf("read source addon %q: %w", source, err)
	}
	if !found {
		return fmt.Errorf("source addon %q is not registered", source)
	}
	if sourceAddon.WorktreeOf != "" {
		return fmt.Errorf("source addon %q is already a worktree of %q", source, sourceAddon.WorktreeOf)
	}
	if sourceAddon.Branch != "" {
		return fmt.Errorf("source addon %q is already registered as a worktree", source)
	}

	if _, found, err := files.LookupAddon(state, name); err != nil {
		return fmt.Errorf("check addon %q: %w", name, err)
	} else if found {
		return fmt.Errorf("addon %q is already registered", name)
	}

	sourcePath := sourceAddon.Path
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("check source addon %q: %w", source, err)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("source addon %q path %q is not a directory", source, sourcePath)
	}
	if err := validateGitRepository(sourcePath); err != nil {
		return fmt.Errorf("source addon %q is not a valid Git repository: %w", source, err)
	}
	if err := validateLocalBranch(sourcePath, branch); err != nil {
		return fmt.Errorf("branch %q cannot be used for source addon %q: %w", branch, source, err)
	}

	destination := filepath.Join(addonsDir, name)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("addon path %q already exists", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check addon path %q: %w", destination, err)
	}

	sourceAbsolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve source addon %q: %w", source, err)
	}
	destinationAbsolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve addon path %q: %w", name, err)
	}
	relativeDestination, err := filepath.Rel(sourceAbsolute, destinationAbsolute)
	if err != nil {
		return fmt.Errorf("resolve worktree path %q: %w", name, err)
	}

	cmd := exec.Command("git", "-C", sourcePath, "worktree", "add", relativeDestination, branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create worktree %q: %w", name, err)
	}

	if err := files.RegisterWorktree(state, name, destination, source, branch); err != nil {
		return fmt.Errorf("register worktree: %w", err)
	}
	return nil
}

func validateGitRepository(path string) error {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != "true" {
		return errors.New("not a non-bare repository")
	}
	return nil
}

func validateLocalBranch(path, branch string) error {
	cmd := exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
		return errors.New("branch does not exist as a local branch")
	}
	return nil
}

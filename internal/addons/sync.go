package addons

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

func Fetch(name string, state files.State) error {
	entry, path, err := syncTarget(name, state)
	if err != nil {
		return err
	}
	if entry.WorktreeOf != "" {
		fmt.Printf("fetching worktree parent repository for addon %q\n", name)
	}
	command := exec.Command("git", "-C", path, "fetch", "--all")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("fetch addon %q: %w", name, err)
	}
	fmt.Printf("addon %q fetched; no profile recreation is required\n", name)
	return nil
}

func Pull(name string, state files.State) error {
	entry, path, err := syncTarget(name, state)
	if err != nil {
		return err
	}
	if entry.WorktreeOf != "" {
		return fmt.Errorf("refusing to pull worktree addon %q; update its parent explicitly", name)
	}

	dirty, err := gitStatusDirty(path)
	if err != nil {
		return fmt.Errorf("inspect addon %q: %w", name, err)
	}
	if dirty {
		return fmt.Errorf("refusing to pull addon %q: repository has uncommitted changes", name)
	}
	branch, err := currentBranch(path)
	if err != nil {
		return fmt.Errorf("inspect addon %q branch: %w", name, err)
	}
	upstream, err := configuredUpstream(path)
	if err != nil {
		return fmt.Errorf("inspect addon %q upstream: %w", name, err)
	}
	if upstream == "" {
		return fmt.Errorf("refusing to pull addon %q: current branch has no configured upstream", name)
	}

	command := exec.Command("git", "-C", path, "pull", "--ff-only")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("pull addon %q: %w", name, err)
	}
	newBranch, err := currentBranch(path)
	if err != nil {
		return fmt.Errorf("verify addon %q branch: %w", name, err)
	}
	if newBranch != branch {
		return fmt.Errorf("refusing pull result for addon %q: branch changed from %q to %q", name, branch, newBranch)
	}

	profiles, err := profile.ProfilesUsingAddon(state, name)
	if err != nil {
		return fmt.Errorf("find profiles using addon %q: %w", name, err)
	}
	if len(profiles) == 0 {
		fmt.Printf("addon %q pulled; no profile recreation is required\n", name)
	} else {
		fmt.Printf("addon %q pulled; source edits are live for profiles: %s (no recreation required)\n", name, strings.Join(profiles, ", "))
	}
	return nil
}

func syncTarget(name string, state files.State) (Entry, string, error) {
	if !validAddonName(name) {
		return Entry{}, "", fmt.Errorf("invalid addon name %q", name)
	}
	entry, found, err := Lookup(state, name)
	if err != nil {
		return Entry{}, "", fmt.Errorf("read addon %q: %w", name, err)
	}
	if !found {
		return Entry{}, "", fmt.Errorf("addon %q is not registered", name)
	}
	path, err := registeredAddonPath(name, entry)
	if err != nil {
		return Entry{}, "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, "", fmt.Errorf("check addon %q: %w", name, err)
	}
	if !info.IsDir() {
		return Entry{}, "", fmt.Errorf("addon %q path %q is not a directory", name, path)
	}
	if err := validateGitRepository(path); err != nil {
		return Entry{}, "", fmt.Errorf("addon %q is not a Git repository: %w", name, err)
	}
	return entry, path, nil
}

func currentBranch(path string) (string, error) {
	command := exec.Command("git", "-C", path, "symbolic-ref", "--quiet", "--short", "HEAD")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.New("repository is in detached HEAD state")
		}
		return "", err
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", errors.New("repository has no current branch")
	}
	return branch, nil
}

func configuredUpstream(path string) (string, error) {
	command := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

package addons

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

// Remove unregisters an addon and removes its checkout. Profile attachment is
// intentionally a separate operation; callers must detach an addon first.
func Remove(name string, yes, force bool, state files.State) error {
	operationOptions := OperationOptions{}
	terminalConfirmation(&operationOptions)
	return RemoveWithOptions(name, yes, force, state, operationOptions)
}

func RemoveWithOptions(name string, yes, force bool, state files.State, operationOptions OperationOptions) error {
	operationOptions = normalizeOperationOptions(operationOptions)
	if err := operationOptions.Context.Err(); err != nil {
		return err
	}
	if !validAddonName(name) {
		return fmt.Errorf("invalid addon name %q", name)
	}

	addon, found, err := lookupAddon(state, name)
	if err != nil {
		return fmt.Errorf("read addon %q: %w", name, err)
	}
	if !found {
		return fmt.Errorf("addon %q is not registered", name)
	}

	profiles, err := profile.ProfilesUsingAddon(state, name)
	if err != nil {
		return fmt.Errorf("find profiles using addon %q: %w", name, err)
	}
	if len(profiles) > 0 {
		return fmt.Errorf("cannot remove addon %q; it is attached to profiles: %s (detach it first)",
			name, strings.Join(profiles, ", "))
	}

	children, err := childWorktrees(state, name)
	if err != nil {
		return fmt.Errorf("find worktrees of addon %q: %w", name, err)
	}
	if len(children) > 0 {
		return fmt.Errorf("cannot remove addon %q; it has registered worktrees: %s (remove them first)",
			name, strings.Join(children, ", "))
	}

	path, err := registeredAddonPath(name, addon)
	if err != nil {
		return err
	}

	if addon.WorktreeOf != "" {
		return removeWorktree(name, addon, path, yes, force, state, operationOptions)
	}
	if addon.Branch != "" {
		return fmt.Errorf("addon %q has worktree metadata but no source addon", name)
	}
	return removeClone(name, path, yes, force, state, operationOptions)
}

func removeClone(name, path string, yes, force bool, state files.State, operationOptions OperationOptions) error {
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("check addon %q: %w", name, err)
		}
		confirmed, err := confirmAddonRemoval(name, "its checkout is already missing", yes, operationOptions)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
		if err := unregisterAddon(state, name); err != nil {
			return fmt.Errorf("unregister addon %q: %w", name, err)
		}
		fmt.Fprintf(operationOptions.Output, "addon %q checkout was missing; registration removed\n", name)
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("addon %q path %q is a symlink; refusing to remove it", name, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("addon %q path %q is not a directory", name, path)
	}

	linked, err := linkedWorktreePathsWithContext(operationOptions.Context, path)
	if err != nil {
		return fmt.Errorf("inspect addon %q worktrees: %w", name, err)
	}
	if len(linked) > 0 {
		return fmt.Errorf("cannot remove addon %q; Git has linked worktrees: %s (remove them first)",
			name, strings.Join(linked, ", "))
	}

	dirty, err := gitStatusDirtyWithContext(operationOptions.Context, path)
	if err != nil {
		return fmt.Errorf("inspect addon %q: %w", name, err)
	}
	if dirty && !force {
		return fmt.Errorf("addon %q has uncommitted changes; use --force to remove it", name)
	}

	confirmed, err := confirmAddonRemoval(name, "delete its checkout", yes, operationOptions)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove addon %q: %w", name, err)
	}
	if err := unregisterAddon(state, name); err != nil {
		return fmt.Errorf("unregister addon %q: %w", name, err)
	}
	fmt.Fprintf(operationOptions.Output, "addon %q removed\n", name)
	return nil
}

func removeWorktree(name string, addon Entry, path string, yes, force bool, state files.State, operationOptions OperationOptions) error {
	if strings.TrimSpace(addon.Branch) == "" {
		return fmt.Errorf("worktree addon %q has no branch", name)
	}

	parent, found, err := lookupAddon(state, addon.WorktreeOf)
	if err != nil {
		return fmt.Errorf("read source addon %q: %w", addon.WorktreeOf, err)
	}
	if !found {
		return fmt.Errorf("source addon %q for worktree %q is not registered", addon.WorktreeOf, name)
	}
	if parent.WorktreeOf != "" || parent.Branch != "" {
		return fmt.Errorf("source addon %q for worktree %q is itself a worktree", addon.WorktreeOf, name)
	}

	parentPath, err := registeredAddonPath(addon.WorktreeOf, parent)
	if err != nil {
		return fmt.Errorf("resolve source addon %q: %w", addon.WorktreeOf, err)
	}
	parentInfo, err := os.Lstat(parentPath)
	if err != nil {
		return fmt.Errorf("check source addon %q: %w", addon.WorktreeOf, err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source addon %q path %q is a symlink; refusing to use it", addon.WorktreeOf, parentPath)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("source addon %q path %q is not a directory", addon.WorktreeOf, parentPath)
	}

	worktrees, err := gitWorktreePathsWithContext(operationOptions.Context, parentPath)
	if err != nil {
		return fmt.Errorf("inspect source addon %q worktrees: %w", addon.WorktreeOf, err)
	}
	if !containsPath(worktrees, path) {
		return fmt.Errorf("worktree addon %q is not registered by Git; refusing to remove its state", name)
	}

	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check worktree %q: %w", name, err)
	}
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("worktree %q path %q is a symlink; refusing to remove it", name, path)
		}
		if !info.IsDir() {
			return fmt.Errorf("worktree %q path %q is not a directory", name, path)
		}
		dirty, err := gitStatusDirtyWithContext(operationOptions.Context, path)
		if err != nil {
			return fmt.Errorf("inspect worktree %q: %w", name, err)
		}
		if dirty && !force {
			return fmt.Errorf("worktree %q has uncommitted changes; use --force to remove it", name)
		}
	}

	confirmed, err := confirmAddonRemoval(name, "remove its Git worktree", yes, operationOptions)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	args := []string{"-C", parentPath, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--", path)
	cmd := exec.CommandContext(operationOptions.Context, "git", args...)
	cmd.Stdout = operationOptions.Output
	cmd.Stderr = operationOptions.ErrorOutput
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remove worktree %q: %w", name, err)
	}
	if err := unregisterAddon(state, name); err != nil {
		return fmt.Errorf("unregister worktree %q: %w", name, err)
	}
	fmt.Fprintf(operationOptions.Output, "worktree addon %q removed; branch %q was preserved\n", name, addon.Branch)
	return nil
}

func registeredAddonPath(name string, addon Entry) (string, error) {
	if strings.TrimSpace(addon.Path) == "" {
		return "", fmt.Errorf("addon %q has no registered path", name)
	}

	addonsInfo, err := os.Lstat(addonsDir)
	if err == nil {
		if addonsInfo.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("addons directory %q is a symlink; refusing to remove addon %q", addonsDir, name)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check addons directory %q: %w", addonsDir, err)
	}

	addonsRoot, err := filepath.Abs(addonsDir)
	if err != nil {
		return "", fmt.Errorf("resolve addons directory: %w", err)
	}
	recorded, err := filepath.Abs(filepath.FromSlash(addon.Path))
	if err != nil {
		return "", fmt.Errorf("resolve registered path for addon %q: %w", name, err)
	}
	// Addon keys and checkout directory names can differ in existing state, so
	// validate the recorded path against the addons root rather than rebuilding
	// it from name. Still require a direct child to avoid deleting arbitrary
	// project directories through corrupt state.
	if samePath(addonsRoot, recorded) || !samePath(filepath.Dir(recorded), addonsRoot) {
		return "", fmt.Errorf("addon %q is registered outside the addons directory; refusing to remove %q", name, addon.Path)
	}
	return recorded, nil
}

func gitStatusDirty(path string) (bool, error) {
	return gitStatusDirtyWithContext(context.Background(), path)
}

func gitStatusDirtyWithContext(ctx context.Context, path string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "status", "--porcelain", "--untracked-files=all", "--ignored")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func linkedWorktreePathsWithContext(ctx context.Context, repoPath string) ([]string, error) {
	worktrees, err := gitWorktreePathsWithContext(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	repoPath, err = filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	linked := make([]string, 0)
	for _, worktree := range worktrees {
		if !samePath(worktree, repoPath) {
			linked = append(linked, worktree)
		}
	}
	return linked, nil
}

func gitWorktreePaths(repoPath string) ([]string, error) {
	return gitWorktreePathsWithContext(context.Background(), repoPath)
}

func gitWorktreePathsWithContext(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}

	worktrees := make([]string, 0)
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		path, err := filepath.Abs(strings.TrimPrefix(line, "worktree "))
		if err != nil {
			return nil, err
		}
		worktrees = append(worktrees, path)
	}
	return worktrees, nil
}

func containsPath(paths []string, target string) bool {
	for _, path := range paths {
		if samePath(path, target) {
			return true
		}
	}
	return false
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func confirmAddonRemoval(name, action string, yes bool, operationOptions OperationOptions) (bool, error) {
	if yes {
		return true, nil
	}
	return requestConfirmation(operationOptions, Confirmation{
		Kind:      ConfirmAddonRemoval,
		AddonName: name,
		Action:    action,
	})
}

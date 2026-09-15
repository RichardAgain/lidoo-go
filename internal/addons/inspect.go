package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

type AddonStatus struct {
	Name             string
	Entry            Entry
	Path             string
	Kind             string
	PathAvailable    bool
	Dirty            bool
	RepositoryError  string
	ParentAvailable  bool
	ParentError      string
	AttachedProfiles []string
}

// List returns sorted status values for every registered add-on.
func List(state files.State) ([]AddonStatus, error) {
	entries, err := loadAddons(state)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	statuses := make([]AddonStatus, 0, len(names))
	for _, name := range names {
		status, err := inspectAddon(state, name, entries[name])
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// Status returns the status for one registered add-on.
func Status(name string, state files.State) (AddonStatus, error) {
	entries, err := loadAddons(state)
	if err != nil {
		return AddonStatus{}, err
	}
	entry, found := entries[name]
	if !found {
		return AddonStatus{}, fmt.Errorf("addon %q is not registered", name)
	}
	return inspectAddon(state, name, entry)
}

func inspectAddon(state files.State, name string, entry Entry) (AddonStatus, error) {
	path, err := filepath.Abs(filepath.FromSlash(entry.Path))
	if err != nil {
		return AddonStatus{}, fmt.Errorf("resolve addon %q path: %w", name, err)
	}
	profiles, err := profile.ProfilesUsingAddon(state, name)
	if err != nil {
		return AddonStatus{}, fmt.Errorf("find profiles using addon %q: %w", name, err)
	}
	status := AddonStatus{
		Name:             name,
		Entry:            entry,
		Path:             path,
		Kind:             "clone",
		AttachedProfiles: profiles,
	}
	if entry.WorktreeOf != "" {
		status.Kind = "worktree"
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			status.RepositoryError = "path is missing"
		} else {
			status.RepositoryError = err.Error()
		}
	} else if !info.IsDir() {
		status.RepositoryError = "path is not a directory"
	} else {
		status.PathAvailable = true
		dirty, err := gitStatusDirty(path)
		if err != nil {
			status.RepositoryError = err.Error()
		} else {
			status.Dirty = dirty
		}
	}

	if entry.WorktreeOf != "" {
		parent, found, err := Lookup(state, entry.WorktreeOf)
		if err != nil {
			return AddonStatus{}, fmt.Errorf("read worktree parent %q: %w", entry.WorktreeOf, err)
		}
		if !found {
			status.ParentError = "parent is not registered"
		} else {
			parentPath, err := filepath.Abs(filepath.FromSlash(parent.Path))
			if err != nil {
				return AddonStatus{}, fmt.Errorf("resolve worktree parent %q: %w", entry.WorktreeOf, err)
			}
			parentInfo, err := os.Stat(parentPath)
			if err != nil {
				status.ParentError = "parent path is unavailable: " + err.Error()
			} else if !parentInfo.IsDir() {
				status.ParentError = "parent path is not a directory"
			} else if worktrees, err := gitWorktreePaths(parentPath); err != nil {
				status.ParentError = err.Error()
			} else if !containsPath(worktrees, path) {
				status.ParentError = "path is not registered by the parent repository"
			} else {
				status.ParentAvailable = true
			}
		}
	}
	return status, nil
}

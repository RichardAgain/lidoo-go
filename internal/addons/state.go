package addons

import (
	"fmt"
	"path/filepath"
	"sort"

	"lidoo/internal/files"
)

const addonsStateKey = "addons"

type addonEntry struct {
	Path       string `json:"path"`
	Source     string `json:"source,omitempty"`
	WorktreeOf string `json:"worktreeOf,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

func loadAddons(state files.State) (map[string]addonEntry, error) {
	entries := make(map[string]addonEntry)
	if _, err := files.ReadSection(state, addonsStateKey, &entries); err != nil {
		return nil, fmt.Errorf("read addons: %w", err)
	}
	if entries == nil {
		entries = make(map[string]addonEntry)
	}
	return entries, nil
}

func saveAddons(state files.State, entries map[string]addonEntry) error {
	return files.WriteSection(state, addonsStateKey, entries)
}

func lookupAddon(state files.State, name string) (addonEntry, bool, error) {
	entries, err := loadAddons(state)
	if err != nil {
		return addonEntry{}, false, err
	}
	entry, ok := entries[name]
	return entry, ok, nil
}

func registerAddon(state files.State, name, source, path string) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	entries[name] = addonEntry{Path: filepath.ToSlash(path), Source: source}
	return saveAddons(state, entries)
}

func registerWorktree(state files.State, name, path, parent, branch string) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	entries[name] = addonEntry{
		Path:       filepath.ToSlash(path),
		WorktreeOf: parent,
		Branch:     branch,
	}
	return saveAddons(state, entries)
}

func unregisterAddon(state files.State, name string) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	if _, ok := entries[name]; !ok {
		return fmt.Errorf("addon %q not found", name)
	}
	delete(entries, name)
	return saveAddons(state, entries)
}

func childWorktrees(state files.State, parent string) ([]string, error) {
	entries, err := loadAddons(state)
	if err != nil {
		return nil, err
	}

	children := make([]string, 0)
	for name, entry := range entries {
		if entry.WorktreeOf == parent {
			children = append(children, name)
		}
	}
	sort.Strings(children)
	return children, nil
}

package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"lidoo/internal/files"
)

const addonsStateKey = "addons"

type Entry struct {
	Path       string `json:"path"`
	Source     string `json:"source,omitempty"`
	WorktreeOf string `json:"worktreeOf,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

type Mount struct {
	Name string
	Path string
}

func loadAddons(state files.State) (map[string]Entry, error) {
	entries := make(map[string]Entry)
	if _, err := files.ReadSection(state, addonsStateKey, &entries); err != nil {
		return nil, fmt.Errorf("read addons: %w", err)
	}
	if entries == nil {
		entries = make(map[string]Entry)
	}
	return entries, nil
}

func saveAddons(state files.State, entries map[string]Entry) error {
	return files.WriteSection(state, addonsStateKey, entries)
}

func Lookup(state files.State, name string) (Entry, bool, error) {
	entries, err := loadAddons(state)
	if err != nil {
		return Entry{}, false, err
	}
	entry, ok := entries[name]
	return entry, ok, nil
}

func lookupAddon(state files.State, name string) (Entry, bool, error) {
	return Lookup(state, name)
}

func ResolveMounts(state files.State, names []string) ([]Mount, error) {
	mounts := make([]Mount, 0, len(names))
	seen := make(map[string]string, len(names))
	for _, name := range names {
		if !validAddonName(name) {
			return nil, fmt.Errorf("invalid addon name %q", name)
		}
		entry, found, err := Lookup(state, name)
		if err != nil {
			return nil, fmt.Errorf("read addon %q: %w", name, err)
		}
		if !found {
			return nil, fmt.Errorf("addon %q is not registered", name)
		}

		path, err := filepath.Abs(filepath.FromSlash(entry.Path))
		if err != nil {
			return nil, fmt.Errorf("resolve addon %q path: %w", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("addon %q path %q does not exist", name, entry.Path)
			}
			return nil, fmt.Errorf("inspect addon %q path %q: %w", name, entry.Path, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("addon %q path %q is not a directory", name, entry.Path)
		}

		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("resolve addon %q path %q: %w", name, entry.Path, err)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return nil, fmt.Errorf("resolve addon %q path %q: %w", name, entry.Path, err)
		}
		if previous, duplicate := seen[resolved]; duplicate {
			return nil, fmt.Errorf("addons %q and %q resolve to the same mount path %q", previous, name, resolved)
		}
		seen[resolved] = name
		mounts = append(mounts, Mount{Name: name, Path: resolved})
	}
	return mounts, nil
}

func registerAddon(state files.State, name, source, path string) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	entries[name] = Entry{Path: filepath.ToSlash(path), Source: source}
	return saveAddons(state, entries)
}

func registerWorktree(state files.State, name, path, parent, branch string) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	entries[name] = Entry{
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

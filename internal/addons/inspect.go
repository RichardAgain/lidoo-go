package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

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

func List(state files.State) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("no addons found")
		return nil
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
			return err
		}
		statuses = append(statuses, status)
	}
	return renderAddonList(statuses)
}

func Status(name string, state files.State) error {
	entries, err := loadAddons(state)
	if err != nil {
		return err
	}
	if name != "" {
		entry, found := entries[name]
		if !found {
			return fmt.Errorf("addon %q is not registered", name)
		}
		status, err := inspectAddon(state, name, entry)
		if err != nil {
			return err
		}
		return renderAddonStatus(status)
	}
	if len(entries) == 0 {
		fmt.Println("no addons found")
		return nil
	}

	names := make([]string, 0, len(entries))
	for addonName := range entries {
		names = append(names, addonName)
	}
	sort.Strings(names)
	for index, addonName := range names {
		status, err := inspectAddon(state, addonName, entries[addonName])
		if err != nil {
			return err
		}
		if index > 0 {
			fmt.Println()
		}
		if err := renderAddonStatus(status); err != nil {
			return err
		}
	}
	return nil
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

func renderAddonList(statuses []AddonStatus) error {
	table := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tTYPE\tSOURCE/PARENT\tBRANCH\tPATH\tATTACHED PROFILES")
	for _, status := range statuses {
		source := status.Entry.Source
		if status.Kind == "worktree" {
			source = status.Entry.WorktreeOf
		}
		if source == "" {
			source = "-"
		}
		branch := status.Entry.Branch
		if branch == "" {
			branch = "-"
		}
		attached := strings.Join(status.AttachedProfiles, ",")
		if attached == "" {
			attached = "-"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", status.Name, status.Kind, source, branch, status.Path, attached)
	}
	return table.Flush()
}

func renderAddonStatus(status AddonStatus) error {
	fmt.Printf("ADDON: %s\n", status.Name)
	fmt.Printf("type: %s\n", status.Kind)
	if status.Entry.Source != "" {
		fmt.Printf("source: %s\n", status.Entry.Source)
	}
	if status.Entry.WorktreeOf != "" {
		fmt.Printf("worktree parent: %s\n", status.Entry.WorktreeOf)
	}
	if status.Entry.Branch != "" {
		fmt.Printf("branch: %s\n", status.Entry.Branch)
	}
	fmt.Printf("path: %s\n", status.Path)
	if status.PathAvailable {
		if status.Dirty {
			fmt.Println("repository: dirty")
		} else {
			fmt.Println("repository: clean")
		}
	} else {
		fmt.Printf("repository: unavailable (%s)\n", status.RepositoryError)
	}
	if status.Kind == "worktree" {
		if status.ParentAvailable {
			fmt.Println("worktree parent status: available")
		} else {
			fmt.Printf("worktree parent status: unavailable (%s)\n", status.ParentError)
		}
	}
	attached := strings.Join(status.AttachedProfiles, ", ")
	if attached == "" {
		attached = "none"
	}
	fmt.Printf("attached profiles: %s\n", attached)
	return nil
}

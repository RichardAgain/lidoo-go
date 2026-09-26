package addons

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"lidoo/internal/files"
)

// Branches returns the short branch names reachable from a registered add-on
// repository: local branches plus remote branches referenced by their short
// name. The result is sorted and de-duplicated so it can drive a chooser.
func Branches(name string, state files.State) ([]string, error) {
	entry, found, err := lookupAddon(state, name)
	if err != nil {
		return nil, fmt.Errorf("read addon %q: %w", name, err)
	}
	if !found {
		return nil, fmt.Errorf("addon %q is not registered", name)
	}

	cmd := exec.Command("git", "-C", entry.Path, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list branches for addon %q: %w", name, err)
	}

	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		ref := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			seen[strings.TrimPrefix(ref, "refs/heads/")] = true
		case strings.HasPrefix(ref, "refs/remotes/"):
			remote := strings.TrimPrefix(ref, "refs/remotes/")
			parts := strings.SplitN(remote, "/", 2)
			if len(parts) != 2 || parts[1] == "HEAD" {
				continue
			}
			seen[parts[1]] = true
		}
	}

	branches := make([]string, 0, len(seen))
	for branch := range seen {
		if branch == "" {
			continue
		}
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches, nil
}

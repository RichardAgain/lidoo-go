package docker

import (
	"fmt"
	"sort"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

// ProfileSummary is the runtime-facing data needed to list a profile.
type ProfileSummary struct {
	Name    string
	State   string
	URL     string
	Version string
}

// ListProfiles returns profiles known by workspace state, Docker, or both.
func ListProfiles(state files.State) ([]ProfileSummary, error) {
	runtime, err := runtimeProfiles()
	if err != nil {
		return nil, fmt.Errorf("find profiles: %w", err)
	}

	names, err := profile.Names(state)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(names)+len(runtime))
	for _, name := range names {
		seen[name] = true
	}
	for name := range runtime {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	profiles := make([]ProfileSummary, 0, len(names))
	for _, name := range names {
		config, found, err := profile.Lookup(state, name)
		if err != nil {
			return nil, fmt.Errorf("read profile %q: %w", name, err)
		}
		if !found {
			config = profile.NewConfig(name)
		}

		summary := ProfileSummary{
			Name:    name,
			State:   "not created",
			URL:     "http://" + profile.Hostname(name),
			Version: profileVersion(config),
		}
		if container, ok := runtime[name]; ok {
			summary.State = container.State
			if container.Version != "" {
				summary.Version = container.Version
			}
		}
		profiles = append(profiles, summary)
	}
	return profiles, nil
}

func parseProfileList(output []byte) ([]ProfileSummary, error) {
	var profiles []ProfileSummary
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
			return nil, fmt.Errorf("invalid profile row from Docker: %q", line)
		}
		name := strings.TrimSpace(fields[0])
		if err := ValidateProfileName(name); err != nil {
			return nil, fmt.Errorf("invalid profile from Docker: %w", err)
		}
		profiles = append(profiles, ProfileSummary{
			Name:  name,
			State: strings.TrimSpace(fields[1]),
			URL:   "http://" + profile.Hostname(name),
		})
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

func profileVersion(config profile.Config) string {
	if config.Version == nil {
		return "-"
	}
	return *config.Version
}

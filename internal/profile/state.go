package profile

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"lidoo/internal/files"
)

const profilesStateKey = "containers"

type Config struct {
	Addons  []string `json:"addons"`
	Prefix  string   `json:"prefix"`
	Version *string  `json:"version,omitempty"`
}

func loadProfiles(state files.State) (map[string]Config, error) {
	profiles := make(map[string]Config)
	if _, err := files.ReadSection(state, profilesStateKey, &profiles); err != nil {
		return nil, fmt.Errorf("read containers: %w", err)
	}
	if profiles == nil {
		profiles = make(map[string]Config)
	}
	return profiles, nil
}

func saveProfiles(state files.State, profiles map[string]Config) error {
	return files.WriteSection(state, profilesStateKey, profiles)
}

func NewConfig(name string) Config {
	return Config{Addons: []string{}, Prefix: name + "__"}
}

func Lookup(state files.State, name string) (Config, bool, error) {
	profiles, err := loadProfiles(state)
	if err != nil {
		return Config{}, false, err
	}
	config, ok := profiles[name]
	return config, ok, nil
}

func Put(state files.State, name string, config Config) error {
	profiles, err := loadProfiles(state)
	if err != nil {
		return err
	}
	profiles[name] = config
	return saveProfiles(state, profiles)
}

func Ensure(state files.State, name string) (bool, error) {
	if strings.TrimSpace(name) == "" {
		return false, errors.New("container name cannot be empty")
	}

	profiles, err := loadProfiles(state)
	if err != nil {
		return false, err
	}
	if _, ok := profiles[name]; ok {
		return false, nil
	}
	profiles[name] = NewConfig(name)
	return true, saveProfiles(state, profiles)
}

func Remove(state files.State, name string) error {
	profiles, err := loadProfiles(state)
	if err != nil {
		return err
	}
	if _, ok := profiles[name]; !ok {
		return nil
	}
	delete(profiles, name)
	return saveProfiles(state, profiles)
}

func Names(state files.State) ([]string, error) {
	profiles, err := loadProfiles(state)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func SetVersion(state files.State, name, version string) error {
	if strings.TrimSpace(version) == "" {
		return errors.New("container version cannot be empty")
	}

	profiles, err := loadProfiles(state)
	if err != nil {
		return err
	}
	config, ok := profiles[name]
	if !ok {
		return fmt.Errorf("container %q not found", name)
	}
	config.Version = &version
	profiles[name] = config
	return saveProfiles(state, profiles)
}

func Addons(state files.State, name string) ([]string, error) {
	config, ok, err := Lookup(state, name)
	if err != nil || !ok {
		return nil, err
	}
	return config.Addons, nil
}

func Prefix(state files.State, name string) (string, error) {
	config, ok, err := Lookup(state, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return name + "__", nil
	}
	return config.Prefix, nil
}

func Version(state files.State, name string) (*string, error) {
	config, ok, err := Lookup(state, name)
	if err != nil || !ok {
		return nil, err
	}
	return config.Version, nil
}

func ProfilesUsingAddon(state files.State, addonName string) ([]string, error) {
	profiles, err := loadProfiles(state)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for name, config := range profiles {
		for _, configuredAddon := range config.Addons {
			if configuredAddon == addonName {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

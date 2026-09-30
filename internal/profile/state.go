package profile

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"lidoo/internal/files"
)

const profilesStateKey = "containers"

const (
	DBFilterModeProfile  = "profile"
	DBFilterModeDisabled = "disabled"
	DBFilterModeCustom   = "custom"
)

type Config struct {
	Addons          []string `json:"addons"`
	Prefix          string   `json:"prefix"`
	Version         *string  `json:"version,omitempty"`
	DBFilterMode    string   `json:"db_filter_mode,omitempty"`
	DBFilterPattern string   `json:"db_filter_pattern,omitempty"`
	AdminPasswd     string   `json:"admin_passwd,omitempty"`
	LANPort         int      `json:"lan_port,omitempty"`
}

// ConfigUpdate uses pointers so a caller can update one setting without
// changing the other persisted profile settings.
type ConfigUpdate struct {
	DBFilterMode    *string
	DBFilterPattern *string
	AdminPasswd     *string
	LANPort         *int
}

// Validate checks a configuration update before it reaches workspace state.
// It enforces the relationship between the database filter mode and the custom
// pattern so presentation layers can surface the same errors as the CLI.
func (update ConfigUpdate) Validate() error {
	modeSet := update.DBFilterMode != nil
	patternSet := update.DBFilterPattern != nil
	passwordSet := update.AdminPasswd != nil
	if !modeSet && !patternSet && !passwordSet && update.LANPort == nil {
		return errors.New("config requires a database filter mode, a filter pattern, an admin password, or a LAN port")
	}
	if update.LANPort != nil {
		if err := (Config{LANPort: *update.LANPort}).Validate(); err != nil {
			return err
		}
	}
	if patternSet && !modeSet {
		return errors.New("a database filter pattern requires custom mode")
	}
	if modeSet {
		switch *update.DBFilterMode {
		case DBFilterModeCustom:
			if !patternSet {
				return errors.New("custom database filter mode requires a pattern")
			}
		case DBFilterModeProfile, DBFilterModeDisabled:
			if patternSet {
				return errors.New("a database filter pattern is only valid with custom mode")
			}
		default:
			return fmt.Errorf("invalid database filter mode %q: use profile, disabled, or custom", *update.DBFilterMode)
		}
	}
	return nil
}

func (config Config) EffectiveDBFilterMode() string {
	if config.DBFilterMode == "" {
		return DBFilterModeProfile
	}
	return config.DBFilterMode
}

func (config Config) Validate() error {
	if config.LANPort < 0 || config.LANPort > 65535 {
		return errors.New("LAN port must be between 0 and 65535 (0 disables publishing)")
	}
	switch config.EffectiveDBFilterMode() {
	case DBFilterModeProfile, DBFilterModeDisabled:
		if config.DBFilterPattern != "" {
			return fmt.Errorf("database filter pattern is only valid with custom mode")
		}
	case DBFilterModeCustom:
		if config.DBFilterPattern == "" {
			return errors.New("custom database filter mode requires a pattern")
		}
		if _, err := regexp.Compile(config.DBFilterPattern); err != nil {
			return fmt.Errorf("invalid database filter pattern: %w", err)
		}
	default:
		return fmt.Errorf("invalid database filter mode %q: use profile, disabled, or custom", config.DBFilterMode)
	}
	if strings.ContainsAny(config.AdminPasswd, "\r\n") {
		return errors.New("admin password cannot contain newline characters")
	}
	return nil
}

func loadProfiles(state files.State) (map[string]Config, error) {
	profiles := make(map[string]Config)
	if _, err := files.ReadSection(state, profilesStateKey, &profiles); err != nil {
		return nil, fmt.Errorf("read containers: %w", err)
	}
	if profiles == nil {
		profiles = make(map[string]Config)
	}
	for name, config := range profiles {
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("invalid configuration for profile %q: %w", name, err)
		}
	}
	return profiles, nil
}

func saveProfiles(state files.State, profiles map[string]Config) error {
	return files.WriteSection(state, profilesStateKey, profiles)
}

func NewConfig(name string) Config {
	return Config{
		Addons:       []string{},
		Prefix:       name + "__",
		DBFilterMode: DBFilterModeProfile,
	}
}

func ValidatePrefix(prefix string) error {
	if len(prefix) > 63 {
		return fmt.Errorf("invalid database prefix %q: maximum length is 63 characters", prefix)
	}
	for _, character := range prefix {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("_.-", character)) {
			return fmt.Errorf("invalid database prefix %q: use letters, numbers, dots, underscores, or hyphens", prefix)
		}
	}
	return nil
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
	if err := config.Validate(); err != nil {
		return err
	}
	profiles, err := loadProfiles(state)
	if err != nil {
		return err
	}
	profiles[name] = config
	return saveProfiles(state, profiles)
}

// UpdateConfig creates a state entry when needed and changes only the fields
// supplied by update. A profile can therefore be configured before its first
// container is run without touching Docker.
func UpdateConfig(state files.State, name string, update ConfigUpdate) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	profiles, err := loadProfiles(state)
	if err != nil {
		return err
	}
	config, ok := profiles[name]
	if !ok {
		config = NewConfig(name)
	}
	if update.DBFilterMode != nil {
		config.DBFilterMode = *update.DBFilterMode
		if *update.DBFilterMode != DBFilterModeCustom && update.DBFilterPattern == nil {
			config.DBFilterPattern = ""
		}
	}
	if update.DBFilterPattern != nil {
		config.DBFilterPattern = *update.DBFilterPattern
	}
	if update.AdminPasswd != nil {
		config.AdminPasswd = *update.AdminPasswd
	}
	if update.LANPort != nil {
		config.LANPort = *update.LANPort
	}
	if err := config.Validate(); err != nil {
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

package profileio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"lidoo/internal/addons"
	"lidoo/internal/files"
	"lidoo/internal/profile"
)

const currentFormatVersion = 1

var versionPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

type Document struct {
	FormatVersion  int      `json:"formatVersion"`
	Name           string   `json:"name"`
	OdooVersion    string   `json:"odooVersion,omitempty"`
	DatabasePrefix string   `json:"databasePrefix"`
	Addons         []string `json:"addons"`
}

func Export(name, destination string, state files.State) error {
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	config, found, err := profile.Lookup(state, name)
	if err != nil {
		return fmt.Errorf("read profile %q: %w", name, err)
	}
	if !found {
		return fmt.Errorf("profile %q is not in workspace state", name)
	}
	if err := profile.ValidatePrefix(config.Prefix); err != nil {
		return err
	}

	document := Document{
		FormatVersion:  currentFormatVersion,
		Name:           name,
		DatabasePrefix: config.Prefix,
		Addons:         append([]string(nil), config.Addons...),
	}
	if config.Version != nil {
		document.OdooVersion = *config.Version
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile export: %w", err)
	}
	data = append(data, '\n')
	if err := writeNewFile(destination, data); err != nil {
		return err
	}
	fmt.Printf("profile %q exported to %q\n", name, destination)
	return nil
}

func Import(source string, overwrite bool, state files.State) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read profile export %q: %w", source, err)
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode profile export %q: %w", source, err)
	}
	if document.FormatVersion != currentFormatVersion {
		return fmt.Errorf("unsupported profile export format version %d", document.FormatVersion)
	}
	if err := profile.ValidateName(document.Name); err != nil {
		return err
	}
	if err := profile.ValidatePrefix(document.DatabasePrefix); err != nil {
		return err
	}
	if document.OdooVersion != "" && !versionPattern.MatchString(document.OdooVersion) {
		return fmt.Errorf("invalid Odoo version %q", document.OdooVersion)
	}
	if _, err := addons.ResolveMounts(state, document.Addons); err != nil {
		return fmt.Errorf("validate profile addons: %w", err)
	}

	previous, found, err := profile.Lookup(state, document.Name)
	if err != nil {
		return fmt.Errorf("check profile %q: %w", document.Name, err)
	}
	if found && !overwrite {
		return fmt.Errorf("profile %q already exists; use --yes to replace it", document.Name)
	}
	config := profile.Config{
		Addons: append([]string(nil), document.Addons...),
		Prefix: document.DatabasePrefix,
	}
	if document.OdooVersion != "" {
		version := document.OdooVersion
		config.Version = &version
	} else if found {
		config.Version = previous.Version
	}
	if err := profile.Put(state, document.Name, config); err != nil {
		return fmt.Errorf("write imported profile %q: %w", document.Name, err)
	}
	fmt.Printf("profile %q imported into workspace state; run it explicitly to create runtime resources\n", document.Name)
	return nil
}

func writeNewFile(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("profile export destination is required")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve profile export destination: %w", err)
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("profile export destination already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect profile export destination: %w", err)
	}
	parent := filepath.Dir(path)
	if info, err := os.Stat(parent); err != nil {
		return fmt.Errorf("inspect profile export directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("profile export parent %q is not a directory", parent)
	}

	temporary, err := os.CreateTemp(parent, ".lidoo-profile-export-*")
	if err != nil {
		return fmt.Errorf("create profile export: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set profile export permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write profile export: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync profile export: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close profile export: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("install profile export: %w", err)
	}
	return nil
}

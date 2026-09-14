package files

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	statePath            = ".lidoo.json"
	stateSchemaKey       = "_schemaVersion"
	currentSchemaVersion = 1
)

type State map[string]json.RawMessage

func ReadState() (State, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return newState(), nil
		}
		return nil, err
	}

	state := make(State)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = make(State)
	}
	if err := migrate(state); err != nil {
		return nil, err
	}
	return state, nil
}

func SaveState(state State) error {
	if state == nil {
		state = newState()
	}
	if err := migrate(state); err != nil {
		return err
	}
	output, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	output = append(output, '\n')

	directory := filepath.Dir(statePath)
	temporary, err := os.CreateTemp(directory, ".lidoo.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary state permissions: %w", err)
	}
	if _, err := io.Copy(temporary, bytes.NewReader(output)); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary state file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary state file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary state file: %w", err)
	}
	if err := os.Rename(temporaryName, statePath); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}

func migrate(state State) error {
	raw, ok := state[stateSchemaKey]
	if !ok {
		state[stateSchemaKey] = json.RawMessage(fmt.Sprintf("%d", currentSchemaVersion))
		return nil
	}
	var version int
	if err := json.Unmarshal(raw, &version); err != nil {
		return fmt.Errorf("invalid state schema version: %w", err)
	}
	if version < 1 || version > currentSchemaVersion {
		return fmt.Errorf("unsupported state schema version %d", version)
	}
	return nil
}

func newState() State {
	state := make(State)
	_ = migrate(state)
	return state
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// ReadSection decodes one top-level state section into destination. It returns
// false without modifying destination when the section does not exist.
func ReadSection(state State, key string, destination any) (bool, error) {
	raw, ok := state[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return true, err
	}
	return true, nil
}

// WriteSection encodes value into one top-level state section.
func WriteSection(state State, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	state[key] = raw
	return nil
}

func DeleteSection(state State, key string) {
	delete(state, key)
}

func CloneState(state State) State {
	clone := make(State, len(state))
	for key, value := range state {
		clone[key] = append([]byte(nil), value...)
	}
	return clone
}

func RestoreState(state, snapshot State) {
	for key := range state {
		delete(state, key)
	}
	for key, value := range snapshot {
		state[key] = append([]byte(nil), value...)
	}
}

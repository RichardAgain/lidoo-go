package files

import (
	"encoding/json"
	"os"
)

const statePath = ".lidoo.json"

type State map[string]json.RawMessage

func ReadState() (State, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	state := make(State)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = make(State)
	}
	return state, nil
}

func SaveState(state State) error {
	output, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(output, '\n'), 0o644)
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

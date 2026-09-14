package profile

import (
	"encoding/json"
	"strings"
	"testing"

	"lidoo/internal/files"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "legacy defaults to profile", config: Config{Prefix: "demo__"}},
		{name: "profile", config: Config{Prefix: "demo__", DBFilterMode: DBFilterModeProfile}},
		{name: "disabled", config: Config{DBFilterMode: DBFilterModeDisabled}},
		{name: "custom", config: Config{DBFilterMode: DBFilterModeCustom, DBFilterPattern: "^demo_[0-9]+$"}},
		{name: "unknown mode", config: Config{DBFilterMode: "other"}, wantErr: true},
		{name: "custom without pattern", config: Config{DBFilterMode: DBFilterModeCustom}, wantErr: true},
		{name: "invalid custom pattern", config: Config{DBFilterMode: DBFilterModeCustom, DBFilterPattern: "["}, wantErr: true},
		{name: "pattern with profile mode", config: Config{DBFilterMode: DBFilterModeProfile, DBFilterPattern: ".*"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestLegacyConfigUsesProfileFilterMode(t *testing.T) {
	var config Config
	if err := json.Unmarshal([]byte(`{"addons":[],"prefix":"demo__"}`), &config); err != nil {
		t.Fatal(err)
	}
	if got := config.EffectiveDBFilterMode(); got != DBFilterModeProfile {
		t.Fatalf("EffectiveDBFilterMode() = %q, want %q", got, DBFilterModeProfile)
	}
}

func TestUpdateConfigPreservesUnrelatedFields(t *testing.T) {
	version := "18"
	state := files.State{}
	original := Config{Addons: []string{"sale"}, Prefix: "demo__", Version: &version, DBFilterMode: DBFilterModeProfile}
	if err := Put(state, "demo", original); err != nil {
		t.Fatal(err)
	}

	pattern := "^demo_[0-9]+$"
	mode := DBFilterModeCustom
	password := "master-secret"
	if err := UpdateConfig(state, "demo", ConfigUpdate{
		DBFilterMode:    &mode,
		DBFilterPattern: &pattern,
		AdminPasswd:     &password,
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := Lookup(state, "demo")
	if err != nil || !ok {
		t.Fatalf("Lookup() = (%+v, %v, %v)", got, ok, err)
	}
	if strings.Join(got.Addons, ",") != "sale" || got.Prefix != original.Prefix || got.Version == nil || *got.Version != version {
		t.Fatalf("unrelated fields changed: %+v", got)
	}
	if got.DBFilterMode != mode || got.DBFilterPattern != pattern || got.AdminPasswd != password {
		t.Fatalf("updated fields = %+v", got)
	}
}

func TestUpdateConfigCreatesProfile(t *testing.T) {
	state := files.State{}
	password := ""
	if err := UpdateConfig(state, "new-profile", ConfigUpdate{AdminPasswd: &password}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Lookup(state, "new-profile")
	if err != nil || !ok {
		t.Fatalf("Lookup() = (%+v, %v, %v)", got, ok, err)
	}
	if got.Prefix != "new-profile__" || got.EffectiveDBFilterMode() != DBFilterModeProfile {
		t.Fatalf("new profile defaults = %+v", got)
	}
}

func TestUpdateConfigRejectsInvalidProfileName(t *testing.T) {
	state := files.State{}
	mode := DBFilterModeDisabled
	if err := UpdateConfig(state, "Not Valid", ConfigUpdate{DBFilterMode: &mode}); err == nil {
		t.Fatal("UpdateConfig() accepted invalid profile name")
	}
}

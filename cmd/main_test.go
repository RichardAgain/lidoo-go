package main

import (
	"reflect"
	"testing"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

func TestConfigureProfile(t *testing.T) {
	version := "18"
	tests := []struct {
		name        string
		profileName string
		mode        string
		pattern     string
		adminPasswd string
		modeSet     bool
		patternSet  bool
		passwordSet bool
		initial     *profile.Config
		want        profile.Config
		wantErr     bool
	}{
		{
			name:        "configures a new profile",
			profileName: "demo",
			mode:        profile.DBFilterModeProfile,
			adminPasswd: "master-secret",
			modeSet:     true,
			passwordSet: true,
			want: profile.Config{
				Addons:       []string{},
				Prefix:       "demo__",
				DBFilterMode: profile.DBFilterModeProfile,
				AdminPasswd:  "master-secret",
			},
		},
		{
			name:        "updates only the admin password",
			profileName: "demo",
			adminPasswd: "new-master-secret",
			passwordSet: true,
			initial: &profile.Config{
				Addons:          []string{"sale"},
				Prefix:          "demo__",
				Version:         &version,
				DBFilterMode:    profile.DBFilterModeCustom,
				DBFilterPattern: `^demo__(dev|staging)$`,
				AdminPasswd:     "old-master-secret",
			},
			want: profile.Config{
				Addons:          []string{"sale"},
				Prefix:          "demo__",
				Version:         &version,
				DBFilterMode:    profile.DBFilterModeCustom,
				DBFilterPattern: `^demo__(dev|staging)$`,
				AdminPasswd:     "new-master-secret",
			},
		},
		{
			name:        "configures a valid custom filter",
			profileName: "demo",
			mode:        profile.DBFilterModeCustom,
			pattern:     `^demo__(dev|staging)$`,
			modeSet:     true,
			patternSet:  true,
			want: profile.Config{
				Addons:          []string{},
				Prefix:          "demo__",
				DBFilterMode:    profile.DBFilterModeCustom,
				DBFilterPattern: `^demo__(dev|staging)$`,
			},
		},
		{
			name:        "disables the database filter",
			profileName: "demo",
			mode:        profile.DBFilterModeDisabled,
			modeSet:     true,
			initial: &profile.Config{
				Addons:          []string{},
				Prefix:          "demo__",
				DBFilterMode:    profile.DBFilterModeCustom,
				DBFilterPattern: `^demo__(dev|staging)$`,
				AdminPasswd:     "master-secret",
			},
			want: profile.Config{
				Addons:       []string{},
				Prefix:       "demo__",
				DBFilterMode: profile.DBFilterModeDisabled,
				AdminPasswd:  "master-secret",
			},
		},
		{
			name:        "rejects an invalid mode",
			profileName: "demo",
			mode:        "unknown",
			modeSet:     true,
			wantErr:     true,
		},
		{
			name:        "rejects a pattern without custom mode",
			profileName: "demo",
			pattern:     `^demo__.*$`,
			patternSet:  true,
			wantErr:     true,
		},
		{
			name:        "rejects custom mode without a pattern",
			profileName: "demo",
			mode:        profile.DBFilterModeCustom,
			modeSet:     true,
			wantErr:     true,
		},
		{
			name:        "rejects a pattern with disabled mode",
			profileName: "demo",
			mode:        profile.DBFilterModeDisabled,
			pattern:     `^demo__.*$`,
			modeSet:     true,
			patternSet:  true,
			wantErr:     true,
		},
		{
			name:        "rejects an invalid custom pattern",
			profileName: "demo",
			mode:        profile.DBFilterModeCustom,
			pattern:     "[",
			modeSet:     true,
			patternSet:  true,
			wantErr:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := files.State{}
			if test.initial != nil {
				if err := profile.Put(state, test.profileName, *test.initial); err != nil {
					t.Fatal(err)
				}
			}

			err := configureProfile(state, test.profileName, test.mode, test.pattern, test.adminPasswd,
				test.modeSet, test.patternSet, test.passwordSet)
			if (err != nil) != test.wantErr {
				t.Fatalf("configureProfile() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}

			got, ok, err := profile.Lookup(state, test.profileName)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("profile %q was not created", test.profileName)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("profile config = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestReadAdminPasswordFromEnv(t *testing.T) {
	t.Setenv("LIDOO_ADMIN_PASSWORD", "master-secret")
	got, err := readAdminPassword()
	if err != nil {
		t.Fatal(err)
	}
	if got != "master-secret" {
		t.Fatalf("password = %q", got)
	}
}

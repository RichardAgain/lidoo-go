package odoo

import (
	"os"
	"path/filepath"
	"testing"

	"lidoo/internal/addons"
	"lidoo/internal/files"
	profiles "lidoo/internal/profile"
)

func TestMigrationLoadModules(t *testing.T) {
	tests := []struct {
		name        string
		openUpgrade string
		want        string
	}{
		{name: "odoo scripts only", openUpgrade: "", want: "base,web"},
		{name: "with openupgrade framework", openUpgrade: "openupgrade", want: "base,web,openupgrade_framework,lidoo_upgrade_19"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := migrationLoadModules(test.openUpgrade); got != test.want {
				t.Fatalf("migrationLoadModules(%q) = %q, want %q", test.openUpgrade, got, test.want)
			}
		})
	}
}

// TestMigrationAddonsPathCommunityOnly pins the `--community-only` contract: the
// addons path is the Community directory alone, so a profile with Enterprise
// attached migrates as Community.
func TestMigrationAddonsPathCommunityOnly(t *testing.T) {
	state := files.State{}
	registerProfile(t, state, "target", []string{"enterprise"})

	got, err := migrationAddonsPath(state, "target", true, "")
	if err != nil {
		t.Fatalf("migrationAddonsPath: %v", err)
	}
	if want := communityAddonsPath; got != want {
		t.Fatalf("migrationAddonsPath(communityOnly) = %q, want %q", got, want)
	}
}

func TestMigrationAddonsPathIncludesProfileAndOpenUpgrade(t *testing.T) {
	state := files.State{}
	registerProfile(t, state, "target", []string{"enterprise", "openupgrade"})

	got, err := migrationAddonsPath(state, "target", false, "openupgrade")
	if err != nil {
		t.Fatalf("migrationAddonsPath: %v", err)
	}
	want := communityAddonsPath + ",/opt/addons/enterprise,/opt/addons/openupgrade"
	if got != want {
		t.Fatalf("migrationAddonsPath = %q, want %q", got, want)
	}
}

// TestMigrationAddonsPathRejectsUnknownOpenUpgrade keeps the migration from
// starting a backup when the OpenUpgrade addon is not registered.
func TestMigrationAddonsPathRejectsUnknownOpenUpgrade(t *testing.T) {
	state := files.State{}
	if _, err := migrationAddonsPath(state, "target", true, "missing"); err == nil {
		t.Fatal("migrationAddonsPath accepted an unregistered --openupgrade addon")
	}
}

// registerProfile registers a profile and its addons in an in-memory state so
// migrationAddonsPath can resolve them without touching the workspace.
func registerProfile(t *testing.T, state files.State, name string, addonNames []string) {
	t.Helper()
	root := t.TempDir()
	entries := map[string]addons.Entry{}
	for _, addonName := range addonNames {
		path := filepath.Join(root, addonName)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create addon dir: %v", err)
		}
		entries[addonName] = addons.Entry{Path: path}
	}
	if err := files.WriteSection(state, "addons", entries); err != nil {
		t.Fatalf("write addons: %v", err)
	}
	if err := profiles.Put(state, name, profiles.Config{Addons: addonNames, Prefix: name + "__"}); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

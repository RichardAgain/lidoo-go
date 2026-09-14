package docker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"lidoo/internal/profile"
)

func TestDatabaseFilterArgs(t *testing.T) {
	tests := []struct {
		name   string
		config profile.Config
		want   []string
	}{
		{
			name:   "legacy profile mode escapes prefix",
			config: profile.Config{Prefix: "demo.+__"},
			want:   []string{"--db-filter=^demo\\.\\+__.*$"},
		},
		{
			name:   "disabled omits filter",
			config: profile.Config{DBFilterMode: profile.DBFilterModeDisabled},
			want:   nil,
		},
		{
			name:   "custom passes exact regex",
			config: profile.Config{DBFilterMode: profile.DBFilterModeCustom, DBFilterPattern: "^demo_(one|two)$"},
			want:   []string{"--db-filter=^demo_(one|two)$"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := databaseFilterArgs(test.config)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("databaseFilterArgs() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestWriteRuntimeConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	path, err := writeRuntimeConfig("demo", "master-secret")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(".lidoo", "demo", "odoo.conf") {
		t.Fatalf("runtime path = %q", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), "[options]\naddons_path = /mnt/extra-addons\ndata_dir = /var/lib/odoo\nadmin_passwd = master-secret\n"; got != want {
		t.Fatalf("runtime config = %q, want %q", got, want)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("runtime config permissions = %o, want 644", got)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("runtime directory permissions = %o, want 755", got)
	}

	cleared, err := writeRuntimeConfig("demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if cleared != "" {
		t.Fatalf("empty password returned runtime path %q", cleared)
	}
	contents, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), "[options]\naddons_path = /mnt/extra-addons\ndata_dir = /var/lib/odoo\n"; got != want {
		t.Fatalf("cleared runtime config = %q, want %q", got, want)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("cleared runtime config permissions = %o, want 644", got)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("cleared runtime directory permissions = %o, want 755", got)
	}
}

func TestWriteRuntimeConfigDoesNotCreateForEmptyPassword(t *testing.T) {
	t.Chdir(t.TempDir())
	path, err := writeRuntimeConfig("demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("empty password returned runtime path %q", path)
	}
	if _, err := os.Stat(filepath.Join(".lidoo", "demo", "odoo.conf")); !os.IsNotExist(err) {
		t.Fatalf("empty password created runtime config, stat error = %v", err)
	}
}

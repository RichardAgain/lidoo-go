package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionOptionsMergesKnownAndDockerfiles(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("docker", "caddy"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Dockerfile.18", "Dockerfile.17", "Dockerfile.16.0", "Dockerfile.notes"} {
		if err := os.WriteFile(filepath.Join("docker", name), []byte("FROM scratch\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	options, err := VersionOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"19", "18", "17", "16.0"}
	if len(options) != len(want) {
		t.Fatalf("versions = %+v, want %v", options, want)
	}
	for index, version := range want {
		if options[index].Version != version {
			t.Fatalf("versions = %+v, want %v", options, want)
		}
	}
	if options[0].Dockerfile {
		t.Fatal("19 has no Dockerfile in this fixture")
	}
	if !options[1].Dockerfile || !options[2].Dockerfile || !options[3].Dockerfile {
		t.Fatalf("expected dockerfiles for 18/17/16.0: %+v", options)
	}
}

func TestVersionOptionsWithoutDockerDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	options, err := VersionOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"19", "18", "17"}
	if len(options) != len(want) {
		t.Fatalf("versions = %+v, want %v", options, want)
	}
	for _, option := range options {
		if option.Dockerfile {
			t.Fatalf("%s should report no dockerfile: %+v", option.Version, option)
		}
	}
}

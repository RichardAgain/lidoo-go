package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionOptionsDiscoversDockerfiles(t *testing.T) {
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
	got := make([]string, 0, len(options))
	for _, option := range options {
		got = append(got, option.Version)
	}
	want := []string{"18", "17", "16.0"}
	if len(got) != len(want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("versions = %v, want %v", got, want)
		}
	}
}

func TestVersionOptionsErrorsWithoutDockerDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := VersionOptions(context.Background()); err == nil {
		t.Fatal("expected an error when docker/ is missing")
	}
}

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreSourcesListsDumps(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("backups", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.zip", "b.dump", "notes.txt"} {
		if err := os.WriteFile(filepath.Join("backups", name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	service := &Service{}
	sources, err := service.RestoreSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %v, want 2", sources)
	}
	for _, source := range sources {
		if filepath.Ext(source) != ".zip" && filepath.Ext(source) != ".dump" {
			t.Fatalf("unexpected source %q", source)
		}
	}
}

func TestRestoreSourcesMissingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	service := &Service{}
	sources, err := service.RestoreSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("sources = %v, want none", sources)
	}
}

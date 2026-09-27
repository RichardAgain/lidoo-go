package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectoryKeepsDirectoriesAndDumps(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.zip", "b.dump", "notes.txt", ".hidden.zip"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := ListDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	if !entries[0].Dir || entries[0].Name != "sub" {
		t.Fatalf("first entry = %+v, want sub directory", entries[0])
	}
	if entries[1].Name != "a.zip" || entries[2].Name != "b.dump" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestListDirectoryMissing(t *testing.T) {
	if _, err := ListDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateFileAddsManagedHost(t *testing.T) {
	path := writeHostsFile(t, "127.0.0.1 localhost\n192.0.2.10 example.test\n")

	changed, err := updateFile(path, "testing.lidoo.test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("updateFile reported no change")
	}

	content := readHostsFile(t, path)
	if !strings.Contains(content, "127.0.0.1") ||
		!strings.Contains(content, "testing.lidoo.test # lidoo:testing.lidoo.test") {
		t.Fatalf("managed host was not added: %q", content)
	}
	if !strings.Contains(content, "192.0.2.10") || !strings.Contains(content, "example.test") {
		t.Fatalf("unrelated host was changed: %q", content)
	}
}

func TestUpdateFileIsIdempotent(t *testing.T) {
	path := writeHostsFile(t, "127.0.0.1 localhost\n")

	if _, err := updateFile(path, "testing.lidoo.test", true); err != nil {
		t.Fatal(err)
	}
	changed, err := updateFile(path, "testing.lidoo.test", true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second update should not change the hosts file")
	}
}

func TestUpdateFileRefusesConflictingHost(t *testing.T) {
	path := writeHostsFile(t, "192.0.2.10 testing.lidoo.test\n")

	if _, err := updateFile(path, "testing.lidoo.test", true); err == nil {
		t.Fatal("expected conflicting host error")
	}
}

func TestUpdateFileRemovesOnlyManagedHost(t *testing.T) {
	path := writeHostsFile(t, "127.0.0.1 localhost\n127.0.0.1 testing.lidoo.test # lidoo:testing.lidoo.test\n192.0.2.10 example.test\n")

	changed, err := updateFile(path, "testing.lidoo.test", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("updateFile reported no change")
	}

	content := readHostsFile(t, path)
	if strings.Contains(content, "testing.lidoo.test") {
		t.Fatalf("managed host was not removed: %q", content)
	}
	if !strings.Contains(content, "192.0.2.10") || !strings.Contains(content, "example.test") {
		t.Fatalf("unrelated host was removed: %q", content)
	}
}

func writeHostsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readHostsFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

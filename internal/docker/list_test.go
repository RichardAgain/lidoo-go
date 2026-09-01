package docker

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseProfileListSortsProfiles(t *testing.T) {
	profiles, err := parseProfileList([]byte("testing\trunning\ndemo\texited\n"))
	if err != nil {
		t.Fatal(err)
	}

	if len(profiles) != 2 {
		t.Fatalf("got %d profiles, want 2", len(profiles))
	}
	if profiles[0] != (profileRow{name: "demo", status: "exited"}) {
		t.Fatalf("first profile = %+v, want demo/exited", profiles[0])
	}
	if profiles[1] != (profileRow{name: "testing", status: "running"}) {
		t.Fatalf("second profile = %+v, want testing/running", profiles[1])
	}
}

func TestRenderProfileList(t *testing.T) {
	profiles := []profileRow{
		{name: "testing", status: "running"},
		{name: "demo", status: "exited"},
	}

	var output bytes.Buffer
	if err := renderProfileList(&output, profiles); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"PROFILE",
		"STATUS",
		"URL",
		"testing",
		"running",
		"http://testing.lidoo.test",
		"demo",
		"exited",
		"http://demo.lidoo.test",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output missing %q: %q", want, output.String())
		}
	}
}

package docker

import (
	"strings"
	"testing"
)

func TestValidateProfileName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "testing", valid: true},
		{name: "dev-18", valid: true},
		{name: "Testing", valid: false},
		{name: "dev_18", valid: false},
		{name: "-dev", valid: false},
		{name: "dev.", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProfileName(tt.name)
			if (err == nil) != tt.valid {
				t.Fatalf("validateProfileName(%q) error = %v, valid = %v", tt.name, err, tt.valid)
			}
		})
	}
}

func TestProfileHostname(t *testing.T) {
	if got, want := profileHostname("testing"), "testing.lidoo.test"; got != want {
		t.Fatalf("profileHostname(testing) = %q, want %q", got, want)
	}
}

func TestTraefikLabels(t *testing.T) {
	labels := traefikLabels("testing")
	labelText := strings.Join(labels, "\n")

	for _, want := range []string{
		"traefik.enable=true",
		"traefik.docker.network=lidoo-net",
		"traefik.http.routers.testing.rule=Host(`testing.lidoo.test`)",
		"traefik.http.routers.testing.entrypoints=web",
		"traefik.http.routers.testing.service=testing",
		"traefik.http.services.testing.loadbalancer.server.port=8069",
	} {
		if !strings.Contains(labelText, want) {
			t.Errorf("Traefik labels missing %q; got %v", want, labels)
		}
	}
}

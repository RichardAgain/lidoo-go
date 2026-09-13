package profile

import (
	"fmt"
	"regexp"
)

const Domain = "lidoo.localhost"

var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func ValidateName(name string) error {
	if len(name) > 63 || !namePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use 1-63 lowercase letters, numbers, and hyphens", name)
	}
	return nil
}

func Hostname(name string) string {
	return name + "." + Domain
}

func ContainerName(name string) string {
	return "lidoo-" + name
}

package docker

import (
	"fmt"
	"strings"
)

var sharedServices = []string{"db", "caddy"}

func EnsureSharedServices(options CommandOptions) error {
	options = normalizeCommandOptions(options)
	statusArgs := []string{"compose", "ps", "--services", "--status", "running"}
	statusArgs = append(statusArgs, sharedServices...)
	output, err := dockerOutputWithOptions(options, statusArgs...)
	if err != nil {
		return fmt.Errorf("check shared services: %w", err)
	}
	if allSharedServicesRunning(string(output)) {
		return nil
	}

	fmt.Fprintln(options.Stdout, "starting shared services")
	upArgs := []string{"compose", "up", "-d"}
	upArgs = append(upArgs, sharedServices...)
	if err := dockerQuietWithOptions(options, upArgs...); err != nil {
		return fmt.Errorf("start shared services: %w", err)
	}
	return nil
}

func allSharedServicesRunning(output string) bool {
	running := make(map[string]bool, len(sharedServices))
	for _, service := range strings.Fields(output) {
		running[service] = true
	}
	for _, service := range sharedServices {
		if !running[service] {
			return false
		}
	}
	return true
}

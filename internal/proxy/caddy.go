package proxy

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

const (
	configPath      = "docker/caddy/Caddyfile"
	caddyContainer  = "lidoo-caddy"
	odooPort        = 8069
	caddyConfigPath = "/etc/caddy/Caddyfile"
)

type Route struct {
	Host      string
	Container string
	Port      int
}

func Routes(state files.State) ([]Route, error) {
	profiles, err := files.ContainerNames(state)
	if err != nil {
		return nil, err
	}

	routes := make([]Route, 0, len(profiles))
	for _, name := range profiles {
		route, err := RouteForProfile(name)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func RouteForProfile(name string) (Route, error) {
	if err := profile.ValidateName(name); err != nil {
		return Route{}, err
	}
	return Route{
		Host:      profile.Hostname(name),
		Container: profile.ContainerName(name),
		Port:      odooPort,
	}, nil
}

func Render(routes []Route) (string, error) {
	routes = append([]Route(nil), routes...)
	for _, route := range routes {
		if err := validateRoute(route); err != nil {
			return "", err
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Host != routes[j].Host {
			return routes[i].Host < routes[j].Host
		}
		if routes[i].Container != routes[j].Container {
			return routes[i].Container < routes[j].Container
		}
		return routes[i].Port < routes[j].Port
	})

	var output strings.Builder
	output.WriteString("{\n")
	output.WriteString("    auto_https off\n")
	output.WriteString("}\n")
	for _, route := range routes {
		fmt.Fprintf(&output, "\n%s {\n    reverse_proxy %s:%d\n}\n", route.Host, route.Container, route.Port)
	}
	return output.String(), nil
}

func WriteConfig(routes []Route) error {
	content, err := Render(routes)
	if err != nil {
		return err
	}
	return writeConfig(content)
}

func Sync(state files.State) error {
	routes, err := Routes(state)
	if err != nil {
		return fmt.Errorf("build Caddy routes: %w", err)
	}

	oldConfig, hadOldConfig, err := readConfig()
	if err != nil {
		return fmt.Errorf("read Caddy configuration: %w", err)
	}
	content, err := Render(routes)
	if err != nil {
		return fmt.Errorf("render Caddy configuration: %w", err)
	}
	if err := writeConfig(content); err != nil {
		return fmt.Errorf("write Caddy configuration: %w", err)
	}

	if err := caddyCommand("validate"); err != nil {
		restoreErr := restoreConfig(oldConfig, hadOldConfig)
		return fmt.Errorf("failed to validate generated Caddy configuration: %w%s", err, restoreSuffix(restoreErr))
	}
	if err := caddyCommand("reload"); err != nil {
		restoreErr := restoreConfig(oldConfig, hadOldConfig)
		if restoreErr == nil {
			restoreErr = caddyCommand("reload")
		}
		return fmt.Errorf("failed to reload Caddy: %w%s", err, restoreSuffix(restoreErr))
	}
	return nil
}

func validateRoute(route Route) error {
	if route.Port <= 0 || route.Port > 65535 {
		return fmt.Errorf("invalid proxy port %d", route.Port)
	}
	if !strings.HasSuffix(route.Host, "."+profile.Domain) {
		return fmt.Errorf("invalid proxy hostname %q", route.Host)
	}
	profileName := strings.TrimSuffix(route.Host, "."+profile.Domain)
	if err := profile.ValidateName(profileName); err != nil {
		return err
	}
	if route.Container != profile.ContainerName(profileName) {
		return fmt.Errorf("invalid proxy container %q for profile %q", route.Container, profileName)
	}
	return nil
}

func readConfig() ([]byte, bool, error) {
	content, err := os.ReadFile(configPath)
	if err == nil {
		return content, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func writeConfig(content string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".Caddyfile-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, configPath)
}

func restoreConfig(content []byte, hadConfig bool) error {
	if !hadConfig {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeConfig(string(content))
}

func restoreSuffix(err error) string {
	if err == nil {
		return ""
	}
	return "; additionally failed to restore previous Caddy configuration: " + err.Error()
}

func caddyCommand(action string) error {
	args := []string{
		"exec", caddyContainer,
		"caddy", action,
		"--config", caddyConfigPath,
		"--adapter", "caddyfile",
	}
	command := exec.Command("docker", args...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(output.String())
		if detail != "" {
			return fmt.Errorf("docker exec %s: %w: %s", action, err, detail)
		}
		return fmt.Errorf("docker exec %s: %w", action, err)
	}
	return nil
}

package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

const (
	networkName        = "lidoo-net"
	databaseEnvFile    = ".env"
	containerNameLabel = "io.lidoo.name"
)

func containerIDs(filter string, all bool) ([]string, error) {
	args := []string{"ps"}
	if all {
		args = append(args, "--all")
	}
	args = append(args, "--quiet", "--filter", filter)

	output, err := dockerOutput(args...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}

func findContainerByName(name string) (bool, error) {
	containers, err := containerIDs("label="+containerNameLabel+"="+name, true)
	if err != nil {
		return false, fmt.Errorf("find container with label %q: %w", name, err)
	}
	return len(containers) > 0, nil
}

func containerIsRunning(name string) (bool, error) {
	containers, err := containerIDs("label="+containerNameLabel+"="+name, false)
	if err != nil {
		return false, fmt.Errorf("check container with label %q: %w", name, err)
	}
	return len(containers) > 0, nil
}

func containerHasTraefikRoute(containerName, profileName string) (bool, error) {
	format := fmt.Sprintf(`{{index .Config.Labels "traefik.http.routers.%s.rule"}}`, profileName)
	output, err := dockerOutput("inspect", "--format", format, containerName)
	if err != nil {
		return false, fmt.Errorf("inspect container %q routing: %w", containerName, err)
	}
	want := "Host(`" + profileHostname(profileName) + "`)"
	return strings.TrimSpace(string(output)) == want, nil
}

func networkExists() error {
	cmd := exec.Command("docker", "network", "inspect", networkName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Docker network %q does not exist", networkName)
	}
	return nil
}

func docker(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerQuiet(args ...string) error {
	cmd := exec.Command("docker", args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if output.Len() > 0 {
			fmt.Fprint(os.Stderr, output.String())
		}
		return err
	}
	return nil
}

func dockerCommandAvailable(args ...string) bool {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

var knownDatabaseNoise = []string{
	"Warn: Can't find .pfb for face 'Courier'",
	"<string>:38: (ERROR/3) Unexpected indentation.",
	"<string>:43: (WARNING/2) Block quote ends without a blank line; unexpected unindent.",
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[[:alpha:]]`)

type cleanDatabaseOutputWriter struct {
	destination io.Writer
	pending     []byte
	mu          sync.Mutex
}

func newCleanDatabaseOutputWriter(destination io.Writer) *cleanDatabaseOutputWriter {
	return &cleanDatabaseOutputWriter{destination: destination}
}

func (w *cleanDatabaseOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, p...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		if err := w.writeLine(w.pending[:index+1]); err != nil {
			return 0, err
		}
		w.pending = w.pending[index+1:]
	}
	return len(p), nil
}

func (w *cleanDatabaseOutputWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.pending) == 0 {
		return nil
	}
	err := w.writeLine(w.pending)
	w.pending = nil
	return err
}

func (w *cleanDatabaseOutputWriter) writeLine(line []byte) error {
	clean := ansiEscape.ReplaceAll(line, nil)
	trimmed := strings.TrimRight(string(clean), "\r\n")
	for _, noise := range knownDatabaseNoise {
		if trimmed == noise {
			return nil
		}
	}
	if strings.Contains(trimmed, " INFO ") && strings.Contains(trimmed, " click_odoo_contrib.") {
		return nil
	}
	_, err := w.destination.Write(clean)
	return err
}

func dockerCleanDatabaseOutput(args ...string) error {
	cmd := exec.Command("docker", args...)
	writer := newCleanDatabaseOutputWriter(os.Stdout)
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	if flushErr := writer.Flush(); err == nil {
		err = flushErr
	}
	return err
}

func dockerOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

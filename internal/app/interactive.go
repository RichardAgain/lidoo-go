package app

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"lidoo/internal/docker"
	"lidoo/internal/odoo"
)

// DatabaseShellCommand validates the selected database and prepares an
// interactive psql process. The caller owns terminal handoff and streams.
func (s *Service) DatabaseShellCommand(ctx context.Context, profileName, databaseName string) (*exec.Cmd, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	command, err := odoo.ShellDatabaseCommand(ctx, profileName, databaseName, state)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return command, nil
}

// ProfileLogsCommand prepares a Docker logs process after validating the
// profile. It is used for terminal-following logs.
func (s *Service) ProfileLogsCommand(ctx context.Context, profileName string, follow bool, tail int) (*exec.Cmd, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	command, err := docker.LogsCommandWithContext(ctx, profileName, follow, tail)
	if err != nil {
		return nil, err
	}
	return command, nil
}

// ProfileLogs returns non-following logs so the TUI can retain them as task
// output instead of taking over the terminal.
func (s *Service) ProfileLogs(ctx context.Context, profileName string, tail int) (string, error) {
	command, err := s.ProfileLogsCommand(ctx, profileName, false, tail)
	if err != nil {
		return "", err
	}
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	output, err := command.Output()
	if err != nil {
		if detail := strings.TrimSpace(diagnostic.String()); detail != "" {
			return "", fmt.Errorf("read profile logs: %w: %s", err, detail)
		}
		return "", fmt.Errorf("read profile logs: %w", err)
	}
	return string(output), nil
}

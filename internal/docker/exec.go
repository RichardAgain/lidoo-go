package docker

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		return 130
	}
	return 1
}

func Exec(container string, command ...string) error {
	return ExecWithOutput(container, os.Stdout, os.Stderr, command...)
}

func ExecWithOptions(container string, options CommandOptions, command ...string) error {
	return ExecWithOutputOptions(container, options, command...)
}

func ExecInteractive(container string, command ...string) error {
	args := make([]string, 0, len(command)+4)
	args = append(args, "exec", "--interactive", "--tty", container)
	args = append(args, command...)
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ExecAsUser(container, user string, command ...string) error {
	return ExecAsUserWithOptions(container, user, CommandOptions{}, command...)
}

func ExecAsUserWithOptions(container, user string, options CommandOptions, command ...string) error {
	options = normalizeCommandOptions(options)
	args := make([]string, 0, len(command)+4)
	args = append(args, "exec", "--user", user, container)
	args = append(args, command...)

	cmd := exec.CommandContext(options.Context, "docker", args...)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	return commandError(options.Context, cmd.Run())
}

func ExecWithOutput(container string, stdout, stderr io.Writer, command ...string) error {
	return ExecWithOutputOptions(container, CommandOptions{Stdout: stdout, Stderr: stderr}, command...)
}

func ExecWithOutputOptions(container string, options CommandOptions, command ...string) error {
	options = normalizeCommandOptions(options)
	args := make([]string, 0, len(command)+2)
	args = append(args, "exec", container)
	args = append(args, command...)

	cmd := exec.CommandContext(options.Context, "docker", args...)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	return commandError(options.Context, cmd.Run())
}

func CopyTo(container, source, destination string) error {
	return CopyToWithOptions(container, source, destination, CommandOptions{})
}

func CopyToWithOptions(container, source, destination string, options CommandOptions) error {
	return copyPathWithOptions(source, container+":"+destination, options)
}

func CopyFrom(container, source, destination string) error {
	return CopyFromWithOptions(container, source, destination, CommandOptions{})
}

func CopyFromWithOptions(container, source, destination string, options CommandOptions) error {
	return copyPathWithOptions(container+":"+source, destination, options)
}

func copyPathWithOptions(source, destination string, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	cmd := exec.CommandContext(options.Context, "docker", "cp", source, destination)
	cmd.Stdout = options.Stdout
	cmd.Stderr = options.Stderr
	return commandError(options.Context, cmd.Run())
}

func commandError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	return err
}

package docker

import (
	"io"
	"os"
	"os/exec"
)

func Exec(container string, command ...string) error {
	return ExecWithOutput(container, os.Stdout, os.Stderr, command...)
}

func ExecWithOutput(container string, stdout, stderr io.Writer, command ...string) error {
	args := make([]string, 0, len(command)+2)
	args = append(args, "exec", container)
	args = append(args, command...)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

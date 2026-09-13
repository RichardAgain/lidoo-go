package docker

import (
	"io"
	"os"
	"os/exec"
)

func Exec(container string, command ...string) error {
	return ExecWithOutput(container, os.Stdout, os.Stderr, command...)
}

func ExecAsUser(container, user string, command ...string) error {
	args := make([]string, 0, len(command)+4)
	args = append(args, "exec", "--user", user, container)
	args = append(args, command...)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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

func CopyTo(container, source, destination string) error {
	return copyPath(source, container+":"+destination)
}

func CopyFrom(container, source, destination string) error {
	return copyPath(container+":"+source, destination)
}

func copyPath(source, destination string) error {
	cmd := exec.Command("docker", "cp", source, destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

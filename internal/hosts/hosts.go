package hosts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/txn2/txeh"
)

const (
	localAddress    = "127.0.0.1"
	managedPrefix   = "lidoo:"
	ElevatedCommand = "--lidoo-hosts-write"
)

// Ensure adds the hostname to the system hosts file. If the file requires
// elevated permissions, the current executable is relaunched through the
// platform's elevation mechanism.
func Ensure(hostname string) (bool, error) {
	changed, err := updateDefault(hostname, true)
	if err == nil {
		return changed, nil
	}
	if !isPermissionError(err) {
		return false, err
	}
	if err := runElevated(hostname, true); err != nil {
		return false, fmt.Errorf("update hosts file requires elevated permissions: %w", err)
	}
	return true, nil
}

// Remove removes the Lidoo-managed entry for hostname from the system hosts
// file. Entries managed by other tools or users are left untouched.
func Remove(hostname string) error {
	_, err := updateDefault(hostname, false)
	if err == nil {
		return nil
	}
	if !isPermissionError(err) {
		return err
	}
	if err := runElevated(hostname, false); err != nil {
		return fmt.Errorf("update hosts file requires elevated permissions: %w", err)
	}
	return nil
}

// RunElevated applies a hosts-file operation for the hidden elevated command
// used when Ensure or Remove has to relaunch the CLI with higher privileges.
func RunElevated(args []string) error {
	if len(args) != 2 {
		return errors.New("hosts write requires add/remove and hostname")
	}

	var add bool
	switch args[0] {
	case "add":
		add = true
	case "remove":
	default:
		return fmt.Errorf("invalid hosts operation %q", args[0])
	}
	_, err := updateDefault(args[1], add)
	return err
}

func updateDefault(hostname string, add bool) (bool, error) {
	hosts, err := txeh.NewHostsDefault()
	if err != nil {
		return false, err
	}
	return updateHosts(hosts, hostname, add)
}

func updateFile(path, hostname string, add bool) (bool, error) {
	hosts, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:  path,
		WriteFilePath: path,
	})
	if err != nil {
		return false, err
	}
	return updateHosts(hosts, hostname, add)
}

func updateHosts(hosts *txeh.Hosts, hostname string, add bool) (bool, error) {
	comment := managedComment(hostname)
	if add {
		found, address, _ := hosts.HostAddressLookup(hostname, txeh.IPFamilyV4)
		if found {
			if address != localAddress {
				return false, fmt.Errorf("hostname %q already points to %s", hostname, address)
			}
			return false, nil
		}
		hosts.AddHostWithComment(localAddress, hostname, comment)
	} else {
		if len(hosts.ListHostsByComment(comment)) == 0 {
			return false, nil
		}
		hosts.RemoveByComment(comment)
	}

	if err := hosts.Save(); err != nil {
		return false, err
	}
	return true, nil
}

func managedComment(hostname string) string {
	return managedPrefix + hostname
}

func isPermissionError(err error) bool {
	if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
		return true
	}
	errorText := strings.ToLower(err.Error())
	return strings.Contains(errorText, "permission denied") ||
		strings.Contains(errorText, "access is denied")
}

func runElevated(hostname string, add bool) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current executable: %w", err)
	}
	action := "remove"
	if add {
		action = "add"
	}

	if runtime.GOOS == "windows" {
		return runElevatedWindows(executable, action, hostname)
	}

	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("sudo not found: %w", err)
	}
	cmd := exec.Command(sudo, executable, ElevatedCommand, action, hostname)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runElevatedWindows(executable, action, hostname string) error {
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		powershell, err = exec.LookPath("pwsh.exe")
		if err != nil {
			return fmt.Errorf("PowerShell not found: %w", err)
		}
	}

	arguments := strings.Join([]string{
		powerShellString(ElevatedCommand),
		powerShellString(action),
		powerShellString(hostname),
	}, ",")
	script := fmt.Sprintf(
		"$p = Start-Process -FilePath %s -ArgumentList @(%s) -Verb RunAs -Wait -PassThru; exit $p.ExitCode",
		powerShellString(executable), arguments,
	)
	cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func powerShellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

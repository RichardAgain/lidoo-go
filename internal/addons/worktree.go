package addons

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"lidoo/internal/files"
)

func Worktree(source, name, branch string, state files.State) error {
	return worktree(source, name, branch, false, state)
}

func WorktreeWithConfirmation(source, name, branch string, yes bool, state files.State) error {
	return worktree(source, name, branch, yes, state)
}

func worktree(source, name, branch string, yes bool, state files.State) error {
	if !validAddonName(name) {
		return fmt.Errorf("invalid addon name %q", name)
	}
	if strings.TrimSpace(branch) == "" {
		return errors.New("branch cannot be empty")
	}

	sourceAddon, found, err := lookupAddon(state, source)
	if err != nil {
		return fmt.Errorf("read source addon %q: %w", source, err)
	}
	if !found {
		return fmt.Errorf("source addon %q is not registered", source)
	}
	if sourceAddon.WorktreeOf != "" {
		return fmt.Errorf("source addon %q is already a worktree of %q", source, sourceAddon.WorktreeOf)
	}
	if sourceAddon.Branch != "" {
		return fmt.Errorf("source addon %q is already registered as a worktree", source)
	}

	if _, found, err := lookupAddon(state, name); err != nil {
		return fmt.Errorf("check addon %q: %w", name, err)
	} else if found {
		return fmt.Errorf("addon %q is already registered", name)
	}

	sourcePath := sourceAddon.Path
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("check source addon %q: %w", source, err)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("source addon %q path %q is not a directory", source, sourcePath)
	}
	if err := validateGitRepository(sourcePath); err != nil {
		return fmt.Errorf("source addon %q is not a valid Git repository: %w", source, err)
	}
	localBranch, err := localBranchExists(sourcePath, branch)
	if err != nil {
		return fmt.Errorf("check branch %q in source addon %q: %w", branch, source, err)
	}

	remoteBranch := ""
	if !localBranch {
		if err := fetchRemotes(sourcePath); err != nil {
			return fmt.Errorf("fetch branches for source addon %q: %w", source, err)
		}
		remoteBranch, err = findRemoteBranch(sourcePath, branch)
		if err != nil {
			return fmt.Errorf("check remote branch %q in source addon %q: %w", branch, source, err)
		}
	}

	destination := filepath.Join(addonsDir, name)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("addon path %q already exists", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check addon path %q: %w", destination, err)
	}

	sourceAbsolute, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve source addon %q: %w", source, err)
	}
	destinationAbsolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve addon path %q: %w", name, err)
	}
	relativeDestination, err := filepath.Rel(sourceAbsolute, destinationAbsolute)
	if err != nil {
		return fmt.Errorf("resolve worktree path %q: %w", name, err)
	}

	branchCreated := false
	if localBranch {
		err = addWorktree(sourcePath, relativeDestination, branch, "", false)
	} else if remoteBranch != "" {
		// Keep the worktree on a local branch while preserving the remote branch
		// as its upstream.
		err = addWorktree(sourcePath, relativeDestination, branch, remoteBranch, true)
		branchCreated = err == nil
	} else {
		// Let Git report the missing branch before offering to create one.
		err = addWorktree(sourcePath, relativeDestination, branch, branch, false)
	}
	if err != nil {
		if localBranch || remoteBranch != "" {
			return fmt.Errorf("create worktree %q: %w", name, err)
		}

		create, promptErr := confirmNewWorktree(branch, yes)
		if promptErr != nil {
			return promptErr
		}
		if !create {
			fmt.Printf("worktree %q was not created; branch %q was not created\n", name, branch)
			return nil
		}
		if err := addWorktree(sourcePath, relativeDestination, branch, "", true); err != nil {
			return fmt.Errorf("create worktree %q with new branch %q: %w", name, branch, err)
		}
		branchCreated = true
	}

	if err := registerWorktree(state, name, destination, source, branch); err != nil {
		return fmt.Errorf("register worktree: %w", err)
	}
	if branchCreated {
		if remoteBranch != "" {
			fmt.Printf("worktree %q created at %q; branch %q created: yes (from remote branch)\n", name, destination, branch)
		} else {
			fmt.Printf("worktree %q created at %q; branch %q created: yes\n", name, destination, branch)
		}
	} else {
		fmt.Printf("worktree %q created at %q; branch created: no (existing local branch)\n", name, destination)
	}
	return nil
}

func addWorktree(sourcePath, destination, branch, startPoint string, createBranch bool) error {
	args := []string{"-C", sourcePath, "worktree", "add"}
	if createBranch {
		args = append(args, "-b", branch)
	}
	args = append(args, destination)
	if startPoint != "" {
		args = append(args, startPoint)
	}

	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func validateGitRepository(path string) error {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != "true" {
		return errors.New("not a non-bare repository")
	}
	return nil
}

func localBranchExists(path, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func fetchRemotes(path string) error {
	cmd := exec.Command("git", "-C", path, "fetch", "--all")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findRemoteBranch(path, branch string) (string, error) {
	cmd := exec.Command("git", "-C", path, "remote")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	for _, remote := range strings.Fields(string(output)) {
		ref := "refs/remotes/" + remote + "/" + branch
		check := exec.Command("git", "-C", path, "show-ref", "--verify", "--quiet", ref)
		if err := check.Run(); err == nil {
			return ref, nil
		} else {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				return "", err
			}
		}
	}
	return "", nil
}

func confirmNewWorktree(branch string, yes bool) (bool, error) {
	if yes {
		return true, nil
	}

	fmt.Fprintf(os.Stderr, "branch %q does not exist locally or remotely; create a new branch and worktree? [Y/N] ", branch)
	var answer string
	if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

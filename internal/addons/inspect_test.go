package addons

import (
	"os/exec"
	"testing"

	"lidoo/internal/files"
)

func initTestRepository(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=Test"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	run("init", "--quiet", "--initial-branch", branch)
	run("commit", "--quiet", "--allow-empty", "-m", "init")
	return dir
}

func TestStatusReportsCurrentBranchOfPlainClone(t *testing.T) {
	dir := initTestRepository(t, "saas-18.1")
	state := files.State{}
	if err := registerAddon(state, "demo", "git@github.com:acme/demo.git", dir); err != nil {
		t.Fatal(err)
	}

	status, err := Status("demo", state)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch != "saas-18.1" {
		t.Fatalf("branch = %q, want saas-18.1", status.Branch)
	}
}

func TestStatusFollowsCheckedOutBranch(t *testing.T) {
	dir := initTestRepository(t, "main")
	state := files.State{}
	if err := registerAddon(state, "demo", "git@github.com:acme/demo.git", dir); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", dir, "checkout", "--quiet", "-b", "fix-1234").CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v: %s", err, output)
	}

	statuses, err := List(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].Branch != "fix-1234" {
		t.Fatalf("branch = %q, want fix-1234", statuses[0].Branch)
	}
}

func TestStatusFallsBackToRecordedBranch(t *testing.T) {
	dir := t.TempDir() // not a Git repository
	state := files.State{}
	if err := registerWorktree(state, "demo-hotfix", dir, "demo", "recorded-branch"); err != nil {
		t.Fatal(err)
	}

	status, err := Status("demo-hotfix", state)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch != "recorded-branch" {
		t.Fatalf("branch = %q, want recorded-branch", status.Branch)
	}
}

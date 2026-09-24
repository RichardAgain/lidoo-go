package addons

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ConfirmationKind identifies an add-on workflow decision that belongs to the
// caller, such as deleting a checkout or creating a worktree branch.
type ConfirmationKind string

const (
	ConfirmAddonRemoval ConfirmationKind = "remove"
	ConfirmNewWorktree  ConfirmationKind = "new-worktree"
)

type Confirmation struct {
	Kind      ConfirmationKind
	AddonName string
	Action    string
}

// OperationOptions carries cancellation, output, and confirmation behavior
// into add-on operations without requiring them to own terminal input.
type OperationOptions struct {
	Context     context.Context
	Output      io.Writer
	ErrorOutput io.Writer
	Confirm     func(Confirmation) (bool, error)
}

func normalizeOperationOptions(options OperationOptions) OperationOptions {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Output == nil {
		options.Output = os.Stdout
	}
	if options.ErrorOutput == nil {
		options.ErrorOutput = os.Stderr
	}
	return options
}

func terminalConfirmation(options *OperationOptions) {
	options.Confirm = func(confirmation Confirmation) (bool, error) {
		if confirmation.Kind == ConfirmNewWorktree {
			fmt.Fprintf(options.ErrorOutput, "branch %q does not exist locally or remotely; create a new branch and worktree? [Y/N] ", confirmation.AddonName)
		} else {
			fmt.Fprintf(options.ErrorOutput, "remove addon %q and %s? [Y/N] ", confirmation.AddonName, confirmation.Action)
		}
		var answer string
		if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
	}
}

func requestConfirmation(options OperationOptions, confirmation Confirmation) (bool, error) {
	if options.Confirm == nil {
		return false, errors.New("confirmation is required")
	}
	return options.Confirm(confirmation)
}

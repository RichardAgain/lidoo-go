package app

import (
	"context"
	"fmt"
	"io"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
)

// AddonOperationOptions keeps output and confirmation at the presentation
// boundary while the application owns workspace transactions.
type AddonOperationOptions struct {
	Output      io.Writer
	ErrorOutput io.Writer
	Confirm     func(AddonConfirmation) (bool, error)
}

type AddonConfirmation = addons.Confirmation

type AddAddonInput struct {
	Name    string
	URL     string
	Options addons.AddOptions
}

type AddonMountInput struct {
	ProfileName string
	AddonNames  []string
	Recreate    bool
}

type RemoveAddonInput struct {
	Name  string
	Yes   bool
	Force bool
}

type WorktreeAddonInput struct {
	Source string
	Name   string
	Branch string
	Yes    bool
}

func normalizeAddonOperationOptions(ctx context.Context, options []AddonOperationOptions) addons.OperationOptions {
	var result AddonOperationOptions
	if len(options) > 0 {
		result = options[0]
	}
	if result.Output == nil {
		result.Output = io.Discard
	}
	if result.ErrorOutput == nil {
		result.ErrorOutput = io.Discard
	}
	return addons.OperationOptions{
		Context:     ctx,
		Output:      result.Output,
		ErrorOutput: result.ErrorOutput,
		Confirm:     result.Confirm,
	}
}

func (s *Service) AddAddon(ctx context.Context, input AddAddonInput, options ...AddonOperationOptions) error {
	operationOptions := normalizeAddonOperationOptions(ctx, options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		return addons.AddWithOptions(input.Name, input.URL, input.Options, state, operationOptions)
	})
}

func (s *Service) AttachAddons(ctx context.Context, input AddonMountInput, options ...AddonOperationOptions) error {
	return s.changeAddonMounts(ctx, input, true, options...)
}

func (s *Service) DetachAddons(ctx context.Context, input AddonMountInput, options ...AddonOperationOptions) error {
	return s.changeAddonMounts(ctx, input, false, options...)
}

func (s *Service) changeAddonMounts(ctx context.Context, input AddonMountInput, attach bool, options ...AddonOperationOptions) error {
	operationOptions := normalizeAddonOperationOptions(ctx, options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		if err := docker.ValidateProfileName(input.ProfileName); err != nil {
			return err
		}
		previousState := files.CloneState(state)
		var err error
		if attach {
			err = addons.AttachToContainerWithOptions(input.ProfileName, input.AddonNames, state, operationOptions)
		} else {
			err = addons.DetachFromContainerWithOptions(input.ProfileName, input.AddonNames, state, operationOptions)
		}
		if err != nil {
			return err
		}

		dockerOptions := docker.CommandOptions{
			Context: ctx,
			Stdout:  operationOptions.Output,
			Stderr:  operationOptions.ErrorOutput,
		}
		exists, err := docker.ProfileExistsWithOptions(input.ProfileName, dockerOptions)
		if err != nil {
			if input.Recreate {
				return fmt.Errorf("inspect profile %q before recreation: %w", input.ProfileName, err)
			}
			fmt.Fprintf(operationOptions.ErrorOutput, "warning: addon mounts for profile %q changed, but its container could not be inspected: %v; recreate it explicitly before they apply\n", input.ProfileName, err)
			return nil
		}
		if !exists {
			fmt.Fprintf(operationOptions.Output, "profile %q has no container; addon mounts apply when it is run\n", input.ProfileName)
			return nil
		}
		if !input.Recreate {
			fmt.Fprintf(operationOptions.Output, "profile %q addon mounts changed; recreate it before they apply (or use --recreate)\n", input.ProfileName)
			return nil
		}

		fmt.Fprintf(operationOptions.Output, "recreating profile %q to apply addon mounts\n", input.ProfileName)
		if err := docker.RecreateWithOptions(input.ProfileName, state, dockerOptions); err != nil {
			files.RestoreState(state, previousState)
			return fmt.Errorf("recreate profile %q after addon change: %w", input.ProfileName, err)
		}
		return nil
	})
}

func (s *Service) FetchAddon(ctx context.Context, name string, options ...AddonOperationOptions) error {
	operationOptions := normalizeAddonOperationOptions(ctx, options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		return addons.FetchWithOptions(name, state, operationOptions)
	})
}

func (s *Service) PullAddon(ctx context.Context, name string, options ...AddonOperationOptions) error {
	operationOptions := normalizeAddonOperationOptions(ctx, options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		return addons.PullWithOptions(name, state, operationOptions)
	})
}

func (s *Service) RemoveAddon(ctx context.Context, input RemoveAddonInput, options ...AddonOperationOptions) error {
	operationOptions := normalizeAddonOperationOptions(ctx, options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		return addons.RemoveWithOptions(input.Name, input.Yes, input.Force, state, operationOptions)
	})
}

func (s *Service) WorktreeAddon(ctx context.Context, input WorktreeAddonInput, options ...AddonOperationOptions) error {
	operationOptions := normalizeAddonOperationOptions(ctx, options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		return addons.WorktreeWithOptions(input.Source, input.Name, input.Branch, input.Yes, state, operationOptions)
	})
}

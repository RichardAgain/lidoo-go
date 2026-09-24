package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/profile"
)

// ProfileOperationOptions keeps terminal concerns at the presentation
// boundary while allowing the TUI to retain operation output in a buffer.
type ProfileOperationOptions struct {
	Output      io.Writer
	ErrorOutput io.Writer
	Confirm     func(ProfileConfirmation) (bool, error)
}

// ProfileConfirmation describes a decision the presentation layer must make.
// The application layer never reads terminal input itself.
type ProfileConfirmation struct {
	ProfileName string
	Action      string
	Description string
}

type RunProfileInput struct {
	Name    string
	Version string
}

// StartProfileInput is kept as the application-layer name for the run
// workflow described by the CLI.
type StartProfileInput = RunProfileInput

type RemoveProfileInput struct {
	Name string
	Yes  bool
}

// ProfileVersionRequiredError tells interactive callers that run needs a
// version only because the workspace has no stored version for this profile.
type ProfileVersionRequiredError struct {
	ProfileName string
}

func (err ProfileVersionRequiredError) Error() string {
	return fmt.Sprintf("run requires --version because container %q has no stored version", err.ProfileName)
}

func IsProfileVersionRequired(err error) bool {
	var required *ProfileVersionRequiredError
	return errors.As(err, &required)
}

func normalizeProfileOperationOptions(options []ProfileOperationOptions) ProfileOperationOptions {
	var result ProfileOperationOptions
	if len(options) > 0 {
		result = options[0]
	}
	if result.Output == nil {
		result.Output = io.Discard
	}
	if result.ErrorOutput == nil {
		result.ErrorOutput = io.Discard
	}
	return result
}

func (s *Service) RunProfile(ctx context.Context, input RunProfileInput, options ...ProfileOperationOptions) error {
	operationOptions := normalizeProfileOperationOptions(options)
	return s.mutateProfile(ctx, true, func(state files.State) error {
		if input.Name == "" {
			return errors.New("run requires name")
		}
		if err := docker.ValidateProfileName(input.Name); err != nil {
			return err
		}
		storedVersion, err := profile.Version(state, input.Name)
		if err != nil {
			return err
		}
		if storedVersion == nil && input.Version == "" {
			return &ProfileVersionRequiredError{ProfileName: input.Name}
		}
		return docker.RunWithOptions(input.Name, input.Version, state, docker.CommandOptions{
			Context: ctx,
			Stdout:  operationOptions.Output,
			Stderr:  operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) StartProfile(ctx context.Context, input StartProfileInput, options ...ProfileOperationOptions) error {
	return s.RunProfile(ctx, RunProfileInput(input), options...)
}

func (s *Service) StopProfile(ctx context.Context, name string, options ...ProfileOperationOptions) error {
	operationOptions := normalizeProfileOperationOptions(options)
	return s.mutateProfile(ctx, false, func(files.State) error {
		return docker.StopWithOptions(name, docker.CommandOptions{
			Context: ctx,
			Stdout:  operationOptions.Output,
			Stderr:  operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) RestartProfile(ctx context.Context, name string, options ...ProfileOperationOptions) error {
	operationOptions := normalizeProfileOperationOptions(options)
	return s.mutateProfile(ctx, false, func(files.State) error {
		return docker.RestartWithOptions(name, docker.CommandOptions{
			Context: ctx,
			Stdout:  operationOptions.Output,
			Stderr:  operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) RecreateProfile(ctx context.Context, name string, options ...ProfileOperationOptions) error {
	operationOptions := normalizeProfileOperationOptions(options)
	return s.mutateProfile(ctx, true, func(state files.State) error {
		return docker.RecreateWithOptions(name, state, docker.CommandOptions{
			Context: ctx,
			Stdout:  operationOptions.Output,
			Stderr:  operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) RemoveProfile(ctx context.Context, input RemoveProfileInput, options ...ProfileOperationOptions) error {
	operationOptions := normalizeProfileOperationOptions(options)
	return s.mutateProfile(ctx, true, func(state files.State) error {
		removeOptions := docker.RemoveOptions{
			CommandOptions: docker.CommandOptions{
				Context: ctx,
				Stdout:  operationOptions.Output,
				Stderr:  operationOptions.ErrorOutput,
			},
			Yes: input.Yes,
		}
		if operationOptions.Confirm != nil {
			removeOptions.Confirm = func() (bool, error) {
				return operationOptions.Confirm(ProfileConfirmation{
					ProfileName: input.Name,
					Action:      "stop and remove the running profile container",
					Description: "The profile workspace entry is removed; PostgreSQL data, the filestore volume, and registered add-on checkouts are kept.",
				})
			}
		}
		return docker.RemoveWithStateOptions(input.Name, state, removeOptions)
	})
}

func (s *Service) mutateProfile(ctx context.Context, save bool, operation func(files.State) error) error {
	return s.withWorkspaceOperation(ctx, save, operation)
}

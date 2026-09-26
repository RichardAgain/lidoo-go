package app

import (
	"context"
	"fmt"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/profile"
)

// Versions lists the Odoo versions the workspace can build, newest first.
func (s *Service) Versions(ctx context.Context) ([]docker.VersionOption, error) {
	options, err := docker.VersionOptions(ctx)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return options, nil
}

// AllAddons returns status data for every registered add-on, not only the ones
// attached to a profile.
func (s *Service) AllAddons(ctx context.Context) ([]addons.AddonStatus, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	statuses, err := addons.List(state)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return statuses, nil
}

// AddonBranches lists the branch names reachable from one registered add-on
// repository.
func (s *Service) AddonBranches(ctx context.Context, name string) ([]string, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	branches, err := addons.Branches(name, state)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return branches, nil
}

// ProfileConfig returns the persisted configuration for one profile. The
// boolean reports whether a stored entry exists.
func (s *Service) ProfileConfig(ctx context.Context, name string) (profile.Config, bool, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return profile.Config{}, false, err
	}
	if err := profile.ValidateName(name); err != nil {
		return profile.Config{}, false, err
	}
	config, found, err := profile.Lookup(state, name)
	if err != nil {
		return profile.Config{}, false, err
	}
	if err := contextError(ctx); err != nil {
		return profile.Config{}, false, err
	}
	return config, found, nil
}

// UpdateProfileConfig applies a validated configuration change to one profile
// without touching Docker.
func (s *Service) UpdateProfileConfig(ctx context.Context, name string, update profile.ConfigUpdate, options ...ProfileOperationOptions) error {
	operationOptions := normalizeProfileOperationOptions(options)
	return s.withWorkspaceOperation(ctx, true, func(state files.State) error {
		if err := profile.ValidateName(name); err != nil {
			return err
		}
		if err := update.Validate(); err != nil {
			return err
		}
		if err := profile.UpdateConfig(state, name, update); err != nil {
			return err
		}
		fmt.Fprintf(operationOptions.Output, "configuration for profile %q updated\n", name)
		return nil
	})
}

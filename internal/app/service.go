package app

import (
	"context"
	"fmt"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/odoo"
	"lidoo/internal/profile"
)

// Service coordinates read-only workspace queries for presentation layers.
// Workspace state is intentionally not retained between calls.
type Service struct{}

// Open validates that the current workspace state can be read and returns a
// stateless application service.
func Open() (*Service, error) {
	if _, err := files.ReadState(); err != nil {
		return nil, fmt.Errorf("open workspace: %w", err)
	}
	return &Service{}, nil
}

// Profiles returns fresh profile summaries from workspace state and Docker.
func (s *Service) Profiles(ctx context.Context) ([]docker.ProfileSummary, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	profiles, err := docker.ListProfiles(state)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return profiles, nil
}

// Profile returns fresh detail data for one profile.
func (s *Service) Profile(ctx context.Context, name string) (docker.ProfileDetail, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return docker.ProfileDetail{}, err
	}
	details, err := docker.ProfileDetails(name, state)
	if err != nil {
		return docker.ProfileDetail{}, err
	}
	if err := contextError(ctx); err != nil {
		return docker.ProfileDetail{}, err
	}
	if len(details) != 1 {
		return docker.ProfileDetail{}, fmt.Errorf("profile %q not found", name)
	}
	return details[0], nil
}

// Databases returns fresh databases for one selected profile.
func (s *Service) Databases(ctx context.Context, profileName string) ([]odoo.Database, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	databases, err := odoo.ListDatabases(profileName, state)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return databases, nil
}

// Addons returns fresh status data for add-ons attached to one selected
// profile. Results retain the sorted order of addons.List.
func (s *Service) Addons(ctx context.Context, profileName string) ([]addons.AddonStatus, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	config, found, err := profile.Lookup(state, profileName)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", profileName, err)
	}
	if !found {
		return []addons.AddonStatus{}, nil
	}

	statuses, err := addons.List(state)
	if err != nil {
		return nil, err
	}
	attached := make(map[string]bool, len(config.Addons))
	for _, name := range config.Addons {
		attached[name] = true
	}
	selected := make([]addons.AddonStatus, 0, len(config.Addons))
	for _, status := range statuses {
		if attached[status.Name] {
			selected = append(selected, status)
		}
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return selected, nil
}

func (s *Service) readState(ctx context.Context) (files.State, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	state, err := files.ReadState()
	if err != nil {
		return nil, fmt.Errorf("read workspace state: %w", err)
	}
	return state, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

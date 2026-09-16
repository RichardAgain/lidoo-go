package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"lidoo/internal/app"
	"lidoo/internal/odoo"
)

func LoadProfilesCmd(ctx context.Context, service *app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return ProfilesFailedMsg{Err: errors.New("workspace service is unavailable")}
		}
		profiles, err := service.Profiles(ctx)
		if err != nil {
			return ProfilesFailedMsg{Err: err}
		}
		return ProfilesLoadedMsg{Profiles: profiles}
	}
}

func LoadProfileDetailCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return ProfileDetailFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		detail, err := service.Profile(ctx, profileName)
		if err != nil {
			return ProfileDetailFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return ProfileDetailLoadedMsg{RequestID: requestID, ProfileName: profileName, Detail: detail}
	}
}

func LoadDatabasesCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return DatabasesFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		databases, err := service.Databases(ctx, profileName)
		if err != nil {
			return DatabasesFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return DatabasesLoadedMsg{RequestID: requestID, ProfileName: profileName, Databases: databases}
	}
}

func LoadDatabaseInfoCmd(ctx context.Context, service *app.Service, profileName string, database odoo.Database, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return DatabaseInfoFailedMsg{
				RequestID:        requestID,
				ProfileName:      profileName,
				DatabasePhysical: database.Physical,
				Err:              errors.New("workspace service is unavailable"),
			}
		}
		info, err := service.Database(ctx, profileName, database.Logical)
		if err != nil {
			return DatabaseInfoFailedMsg{
				RequestID:        requestID,
				ProfileName:      profileName,
				DatabasePhysical: database.Physical,
				Err:              err,
			}
		}
		return DatabaseInfoLoadedMsg{
			RequestID:        requestID,
			ProfileName:      profileName,
			DatabasePhysical: database.Physical,
			Info:             info,
		}
	}
}

func LoadAddonsCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return AddonsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		addons, err := service.Addons(ctx, profileName)
		if err != nil {
			return AddonsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return AddonsLoadedMsg{RequestID: requestID, ProfileName: profileName, Addons: addons}
	}
}

package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"lidoo/internal/app"
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

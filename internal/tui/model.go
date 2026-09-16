package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lidoo/internal/app"
	"lidoo/internal/docker"
)

type Model struct {
	ctx      context.Context
	service  *app.Service
	profiles list.Model
	loading  bool
	err      error
}

type profileItem struct {
	profile docker.ProfileSummary
}

func (item profileItem) Title() string {
	return item.profile.Name
}

func (item profileItem) Description() string {
	return fmt.Sprintf("%s  %s", item.profile.State, item.profile.URL)
}

func (item profileItem) FilterValue() string {
	return item.profile.Name
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D78F2"})
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#F97068"})
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#667085", Dark: "#98A2B3"})
)

func NewModel(ctx context.Context, service *app.Service) *Model {
	if ctx == nil {
		ctx = context.Background()
	}
	delegate := list.NewDefaultDelegate()
	profiles := list.New(nil, delegate, 80, 24)
	profiles.Title = "Profiles"
	profiles.SetShowFilter(false)
	profiles.SetShowStatusBar(false)
	profiles.SetShowPagination(false)
	profiles.SetShowHelp(false)
	return &Model{
		ctx:      ctx,
		service:  service,
		profiles: profiles,
		loading:  true,
	}
}

func (m *Model) Init() tea.Cmd {
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.profiles.SetSize(msg.Width, msg.Height)
	case ProfilesLoadedMsg:
		m.loading = false
		m.err = nil
		items := make([]list.Item, 0, len(msg.Profiles))
		for _, profile := range msg.Profiles {
			items = append(items, profileItem{profile: profile})
		}
		return m, m.profiles.SetItems(items)
	case ProfilesFailedMsg:
		m.loading = false
		m.err = msg.Err
		return m, nil
	}

	var cmd tea.Cmd
	m.profiles, cmd = m.profiles.Update(msg)
	return m, cmd
}

func (m *Model) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(1, 2).Render(
			titleStyle.Render("Profiles") + "\n\n" + mutedStyle.Render("Loading profiles...") + "\n\n" + mutedStyle.Render("q quit"),
		)
	}
	if m.err != nil {
		return lipgloss.NewStyle().Padding(1, 2).Render(
			titleStyle.Render("Profiles") + "\n\n" + errorStyle.Render(m.err.Error()) + "\n\n" + mutedStyle.Render("q quit"),
		)
	}
	return m.profiles.View()
}

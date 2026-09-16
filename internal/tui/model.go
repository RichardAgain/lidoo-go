package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lidoo/internal/app"
	"lidoo/internal/docker"
)

type focusArea uint8

const (
	focusProfiles focusArea = iota
	focusDatabases
	focusAddons
)

type Model struct {
	ctx           context.Context
	service       *app.Service
	profiles      []docker.ProfileSummary
	profileIndex  int
	databaseIndex int
	addonIndex    int
	focus         focusArea
	loading       bool
	err           error
	status        string
	showHelp      bool
	width         int
	height        int
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D78F2"})
	activeStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#4338CA", Dark: "#A5B4FC"})
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#F97068"})
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#667085", Dark: "#98A2B3"})
	runningStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#027A48", Dark: "#6CE9A6"})
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B54708", Dark: "#FEC84B"})
)

func NewModel(ctx context.Context, service *app.Service) *Model {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Model{
		ctx:     ctx,
		service: service,
		loading: true,
		status:  "Loading profiles",
		width:   80,
		height:  24,
	}
}

func (m *Model) Init() tea.Cmd {
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case ProfilesLoadedMsg:
		m.setProfiles(msg.Profiles)
		m.loading = false
		m.err = nil
		m.status = "Ready"
		return m, nil
	case ProfilesFailedMsg:
		m.loading = false
		m.err = msg.Err
		m.status = "Profile refresh failed"
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "right", "l":
		m.focus = (m.focus + 1) % 3
		return m, nil
	case "shift+tab", "left", "h":
		m.focus = (m.focus + 2) % 3
		return m, nil
	case "up", "k":
		m.moveSelection(-1)
		return m, nil
	case "down", "j":
		m.moveSelection(1)
		return m, nil
	case "r", "ctrl+r":
		m.loading = true
		m.err = nil
		m.status = "Refreshing profiles"
		return m, LoadProfilesCmd(m.ctx, m.service)
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) setProfiles(profiles []docker.ProfileSummary) {
	selected := ""
	if profile := m.selectedProfile(); profile != nil {
		selected = profile.Name
	}
	m.profiles = append([]docker.ProfileSummary(nil), profiles...)
	m.profileIndex = 0
	for index, profile := range m.profiles {
		if profile.Name == selected {
			m.profileIndex = index
			break
		}
	}
	m.databaseIndex = 0
	m.addonIndex = 0
}

func (m *Model) selectedProfile() *docker.ProfileSummary {
	if m.profileIndex < 0 || m.profileIndex >= len(m.profiles) {
		return nil
	}
	return &m.profiles[m.profileIndex]
}

func (m *Model) moveSelection(delta int) {
	var index *int
	count := 0
	switch m.focus {
	case focusProfiles:
		index = &m.profileIndex
		count = len(m.profiles)
	case focusDatabases:
		index = &m.databaseIndex
	case focusAddons:
		index = &m.addonIndex
	}
	if count == 0 || index == nil {
		return
	}
	*index += delta
	if *index < 0 {
		*index = 0
	}
	if *index >= count {
		*index = count - 1
	}
}

func (m *Model) View() string {
	if m.width < 72 || m.height < 14 {
		return m.smallView()
	}

	leftWidth := m.width / 3
	if leftWidth < 24 {
		leftWidth = 24
	}
	if leftWidth > 40 {
		leftWidth = 40
	}
	rightWidth := m.width - leftWidth - 3
	if rightWidth < 24 {
		return m.smallView()
	}

	left := lipgloss.NewStyle().Width(leftWidth).Render(m.leftView(leftWidth))
	right := lipgloss.NewStyle().Width(rightWidth).Render(m.rightView())
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	footer := m.footerView()
	return main + "\n" + mutedStyle.Render(strings.Repeat("─", m.width)) + "\n" + footer
}

func (m *Model) leftView(width int) string {
	profileTitle := titleStyle.Render("Profiles")
	profileRows := make([]string, 0, len(m.profiles))
	for index, profile := range m.profiles {
		name := profile.Name
		state := profileState(profile.State)
		row := name
		if state != "" {
			row += "  " + state
		}
		profileRows = append(profileRows, m.row(row, index == m.profileIndex && m.focus == focusProfiles))
	}
	if len(profileRows) == 0 {
		profileRows = append(profileRows, mutedStyle.Render("  no profiles found"))
	}

	selectedName := "none"
	if profile := m.selectedProfile(); profile != nil {
		selectedName = profile.Name
	}
	sections := []string{
		profileTitle,
		strings.Join(profileRows, "\n"),
		mutedStyle.Render(strings.Repeat("─", width)),
		titleStyle.Render("Databases: " + selectedName),
		mutedStyle.Render("  details load after profile selection"),
		mutedStyle.Render(strings.Repeat("─", width)),
		titleStyle.Render("Add-ons: " + selectedName),
		mutedStyle.Render("  details load after profile selection"),
	}
	return strings.Join(sections, "\n")
}

func (m *Model) rightView() string {
	lines := []string{titleStyle.Render("Selected profile")}
	profile := m.selectedProfile()
	if profile == nil {
		lines = append(lines, mutedStyle.Render("No profile selected"))
	} else {
		lines = append(lines,
			activeStyle.Render(profile.Name),
			"state: "+profileState(profile.State),
			"url: "+profile.URL,
		)
		if profile.Version != "" && profile.Version != "-" {
			lines = append(lines, "odoo version: "+profile.Version)
		}
	}
	if m.err != nil {
		lines = append(lines, "", errorStyle.Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) row(value string, selected bool) string {
	if !selected {
		return "  " + value
	}
	return activeStyle.Render("▸ " + value)
}

func (m *Model) footerView() string {
	status := m.status
	if status == "" {
		status = "Ready"
	}
	keys := "tab focus  ↑/↓ move  r refresh  ? help  q quit"
	if m.showHelp {
		keys = "tab/←/→ focus  ↑/↓/j/k move  r refresh  q quit  ? hide help"
	}
	return mutedStyle.Render(status) + "  " + keys
}

func (m *Model) smallView() string {
	lines := []string{titleStyle.Render("Lidoo"), mutedStyle.Render("Terminal too small for the full layout")}
	if m.err != nil {
		lines = append(lines, errorStyle.Render(m.err.Error()))
	}
	if len(m.profiles) > 0 {
		lines = append(lines, "", titleStyle.Render("Profiles"))
		for index, profile := range m.profiles {
			lines = append(lines, m.row(profile.Name+"  "+profileState(profile.State), index == m.profileIndex && m.focus == focusProfiles))
		}
	}
	lines = append(lines, "", mutedStyle.Render("r refresh  ? help  q quit"))
	if m.showHelp {
		lines = append(lines, mutedStyle.Render("tab focus  arrows or j/k move"))
	}
	return strings.Join(lines, "\n")
}

func profileState(state string) string {
	switch strings.ToLower(state) {
	case "running", "healthy":
		return runningStyle.Render(state)
	case "", "not created":
		return mutedStyle.Render(state)
	default:
		return warningStyle.Render(state)
	}
}

package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lidoo/internal/addons"
	"lidoo/internal/app"
	"lidoo/internal/docker"
	"lidoo/internal/odoo"
)

type focusArea uint8

const (
	focusProfiles focusArea = iota
	focusDatabases
	focusAddons
)

type resourcePhase uint8

const (
	phaseIdle resourcePhase = iota
	phaseLoading
	phaseReady
	phaseEmpty
	phaseUnavailable
	phaseError
)

type Model struct {
	ctx                    context.Context
	service                *app.Service
	profiles               []docker.ProfileSummary
	profileIndex           int
	databases              []odoo.Database
	databaseIndex          int
	addons                 []addons.AddonStatus
	addonIndex             int
	focus                  focusArea
	loading                bool
	err                    error
	status                 string
	showHelp               bool
	width                  int
	height                 int
	profileRequest         uint64
	profileResourceName    string
	profilePhase           resourcePhase
	profileDetail          docker.ProfileDetail
	profileDetailErr       error
	databasePhase          resourcePhase
	databaseErr            error
	databaseRequest        uint64
	databaseInfoPhase      resourcePhase
	databaseInfo           odoo.DatabaseInfo
	databaseInfoErr        error
	addonPhase             resourcePhase
	addonErr               error
	watchEvents            <-chan app.ProfileInvalidation
	watchErrors            <-chan error
	watcherStarted         bool
	watcherConnected       bool
	profileListOnlyRefresh bool
	pendingDatabaseRefresh bool
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
		ctx:           ctx,
		service:       service,
		loading:       true,
		status:        "Loading profiles",
		width:         80,
		height:        24,
		profilePhase:  phaseLoading,
		databasePhase: phaseIdle,
		addonPhase:    phaseIdle,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(LoadProfilesCmd(m.ctx, m.service), StartProfileWatcherCmd(m.ctx, m.service))
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
		listOnly := m.profileListOnlyRefresh
		m.profileListOnlyRefresh = false
		m.setProfiles(msg.Profiles)
		m.loading = false
		m.err = nil
		m.status = "Ready"
		if listOnly && m.selectedProfileName() == m.profileResourceName {
			return m, nil
		}
		return m, m.beginProfileResources()
	case ProfilesFailedMsg:
		m.profileListOnlyRefresh = false
		m.loading = false
		m.err = msg.Err
		m.status = "Profile refresh failed"
		return m, nil
	case ProfileWatcherStartedMsg:
		m.watchEvents = msg.Events
		m.watchErrors = msg.Errors
		m.watcherStarted = true
		m.watcherConnected = true
		return m, WaitProfileInvalidationCmd(m.watchEvents, m.watchErrors)
	case ProfileInvalidationMsg:
		return m, tea.Batch(WaitProfileInvalidationCmd(m.watchEvents, m.watchErrors), m.handleInvalidation(msg.Invalidation))
	case ProfileWatcherDisconnectedMsg:
		m.watcherConnected = false
		m.status = "Docker event watcher disconnected"
		return m, nil
	case ProfileDetailLoadedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.profilePhase = phaseReady
		m.profileDetail = msg.Detail
		m.profileDetailErr = nil
		m.updateProfileSummary(msg.Detail)
		return m, m.afterProfileDetailLoaded()
	case ProfileDetailFailedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.pendingDatabaseRefresh = false
		m.profilePhase = phaseError
		m.profileDetailErr = msg.Err
		m.status = "Profile details unavailable"
		return m, nil
	case DatabasesLoadedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		selected := m.selectedDatabasePhysical()
		m.databases = append([]odoo.Database(nil), msg.Databases...)
		m.databaseIndex = 0
		for index, database := range m.databases {
			if database.Physical == selected {
				m.databaseIndex = index
				break
			}
		}
		m.databaseErr = nil
		if len(m.databases) == 0 {
			m.databasePhase = phaseEmpty
			m.databaseInfoPhase = phaseEmpty
			m.databaseInfoErr = nil
			return m, nil
		}
		m.databasePhase = phaseReady
		m.status = "Ready"
		return m, m.beginDatabaseInfoLoad()
	case DatabasesFailedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.databaseErr = msg.Err
		m.databaseInfoErr = msg.Err
		m.databaseInfoPhase = phaseUnavailable
		if m.profileIsRunning() {
			m.databasePhase = phaseError
			m.status = "Database refresh failed"
		} else {
			m.databasePhase = phaseUnavailable
			m.status = "Databases unavailable while profile is stopped"
		}
		return m, nil
	case DatabaseInfoLoadedMsg:
		if !m.currentDatabaseRequest(msg.RequestID, msg.ProfileName, msg.DatabasePhysical) {
			return m, nil
		}
		m.databaseInfoPhase = phaseReady
		m.databaseInfo = msg.Info
		m.databaseInfoErr = nil
		return m, nil
	case DatabaseInfoFailedMsg:
		if !m.currentDatabaseRequest(msg.RequestID, msg.ProfileName, msg.DatabasePhysical) {
			return m, nil
		}
		m.databaseInfoPhase = phaseError
		m.databaseInfoErr = msg.Err
		m.status = "Database details unavailable"
		return m, nil
	case AddonsLoadedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.addons = append([]addons.AddonStatus(nil), msg.Addons...)
		m.addonIndex = 0
		m.addonErr = nil
		if len(m.addons) == 0 {
			m.addonPhase = phaseEmpty
		} else {
			m.addonPhase = phaseReady
		}
		return m, nil
	case AddonsFailedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.addonPhase = phaseError
		m.addonErr = msg.Err
		m.status = "Add-on refresh failed"
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
		return m, m.moveFocusedSelection(-1)
	case "down", "j":
		return m, m.moveFocusedSelection(1)
	case "r", "ctrl+r":
		m.profileListOnlyRefresh = false
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
	selected := m.selectedProfileName()
	m.profiles = append([]docker.ProfileSummary(nil), profiles...)
	m.profileIndex = 0
	for index, profile := range m.profiles {
		if profile.Name == selected {
			m.profileIndex = index
			break
		}
	}
}

func (m *Model) beginProfileResources() tea.Cmd {
	m.pendingDatabaseRefresh = false
	m.profileRequest++
	m.databaseRequest++
	m.profilePhase = phaseLoading
	m.profileDetail = docker.ProfileDetail{}
	m.profileDetailErr = nil
	m.databasePhase = phaseLoading
	m.databaseErr = nil
	m.databaseInfoPhase = phaseIdle
	m.databaseInfoErr = nil
	m.databases = nil
	m.databaseIndex = 0
	m.addonPhase = phaseLoading
	m.addonErr = nil
	m.addons = nil
	m.addonIndex = 0

	profileName := m.selectedProfileName()
	m.profileResourceName = profileName
	if profileName == "" {
		m.profilePhase = phaseEmpty
		m.databasePhase = phaseEmpty
		m.databaseInfoPhase = phaseEmpty
		m.addonPhase = phaseEmpty
		return nil
	}
	return tea.Batch(
		LoadProfileDetailCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadDatabasesCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadAddonsCmd(m.ctx, m.service, profileName, m.profileRequest),
	)
}

func (m *Model) handleInvalidation(invalidation app.ProfileInvalidation) tea.Cmd {
	if invalidation.ProfileName == "" {
		return nil
	}
	if invalidation.ProfileName != m.selectedProfileName() {
		m.profileListOnlyRefresh = true
		m.status = "Refreshing " + invalidation.ProfileName
		return LoadProfilesCmd(m.ctx, m.service)
	}
	m.status = "Refreshing " + invalidation.ProfileName
	m.profileListOnlyRefresh = false
	m.pendingDatabaseRefresh = invalidation.Kind == app.ProfileRuntimeChanged
	m.profileRequest++
	m.profileResourceName = invalidation.ProfileName
	m.databaseRequest++
	m.profilePhase = phaseLoading
	m.profileDetailErr = nil
	if invalidation.Kind == app.ProfileRuntimeUnavailable {
		m.databasePhase = phaseUnavailable
		m.databaseInfoPhase = phaseUnavailable
	} else {
		m.databasePhase = phaseLoading
		m.databaseInfoPhase = phaseIdle
		m.databaseErr = nil
		m.databaseInfoErr = nil
	}
	return LoadProfileDetailCmd(m.ctx, m.service, invalidation.ProfileName, m.profileRequest)
}

func (m *Model) afterProfileDetailLoaded() tea.Cmd {
	if !m.pendingDatabaseRefresh {
		return nil
	}
	m.pendingDatabaseRefresh = false
	if !strings.EqualFold(m.profileDetail.DockerState, "running") {
		m.databasePhase = phaseUnavailable
		m.databaseInfoPhase = phaseUnavailable
		m.status = "Databases unavailable while profile is stopped"
		return nil
	}
	m.databasePhase = phaseLoading
	m.databaseInfoPhase = phaseIdle
	m.databaseErr = nil
	m.databaseInfoErr = nil
	return LoadDatabasesCmd(m.ctx, m.service, m.selectedProfileName(), m.profileRequest)
}

func (m *Model) updateProfileSummary(detail docker.ProfileDetail) {
	for index := range m.profiles {
		if m.profiles[index].Name != detail.Name {
			continue
		}
		m.profiles[index].State = detail.DockerState
		m.profiles[index].URL = detail.URL
		m.profiles[index].Version = detail.OdooVersion
		return
	}
}

func (m *Model) beginDatabaseInfoLoad() tea.Cmd {
	m.databaseRequest++
	m.databaseInfoErr = nil
	m.databaseInfoPhase = phaseLoading
	database := m.selectedDatabase()
	if database == nil {
		m.databaseInfoPhase = phaseEmpty
		return nil
	}
	return LoadDatabaseInfoCmd(m.ctx, m.service, m.selectedProfileName(), *database, m.databaseRequest)
}

func (m *Model) moveFocusedSelection(delta int) tea.Cmd {
	changed := false
	switch m.focus {
	case focusProfiles:
		changed = m.moveIndex(&m.profileIndex, len(m.profiles), delta)
		if changed {
			m.profileListOnlyRefresh = false
			return m.beginProfileResources()
		}
	case focusDatabases:
		changed = m.moveIndex(&m.databaseIndex, len(m.databases), delta)
		if changed {
			return m.beginDatabaseInfoLoad()
		}
	case focusAddons:
		m.moveIndex(&m.addonIndex, len(m.addons), delta)
	}
	return nil
}

func (m *Model) moveIndex(index *int, count, delta int) bool {
	if count == 0 {
		return false
	}
	old := *index
	*index += delta
	if *index < 0 {
		*index = 0
	}
	if *index >= count {
		*index = count - 1
	}
	return old != *index
}

func (m *Model) currentProfileRequest(requestID uint64, profileName string) bool {
	return requestID == m.profileRequest && profileName != "" && profileName == m.selectedProfileName()
}

func (m *Model) currentDatabaseRequest(requestID uint64, profileName, databasePhysical string) bool {
	database := m.selectedDatabase()
	return requestID == m.databaseRequest && profileName == m.selectedProfileName() && database != nil && database.Physical == databasePhysical
}

func (m *Model) selectedProfile() *docker.ProfileSummary {
	if m.profileIndex < 0 || m.profileIndex >= len(m.profiles) {
		return nil
	}
	return &m.profiles[m.profileIndex]
}

func (m *Model) selectedProfileName() string {
	profile := m.selectedProfile()
	if profile == nil {
		return ""
	}
	return profile.Name
}

func (m *Model) selectedDatabase() *odoo.Database {
	if m.databaseIndex < 0 || m.databaseIndex >= len(m.databases) {
		return nil
	}
	return &m.databases[m.databaseIndex]
}

func (m *Model) selectedDatabasePhysical() string {
	database := m.selectedDatabase()
	if database == nil {
		return ""
	}
	return database.Physical
}

func (m *Model) selectedAddon() *addons.AddonStatus {
	if m.addonIndex < 0 || m.addonIndex >= len(m.addons) {
		return nil
	}
	return &m.addons[m.addonIndex]
}

func (m *Model) profileIsRunning() bool {
	profile := m.selectedProfile()
	return profile != nil && strings.EqualFold(profile.State, "running")
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
	profileRows := make([]string, 0, len(m.profiles))
	for index, profile := range m.profiles {
		row := profile.Name + "  " + profileState(profile.State)
		profileRows = append(profileRows, m.row(row, index == m.profileIndex && m.focus == focusProfiles))
	}
	if len(profileRows) == 0 {
		profileRows = append(profileRows, mutedStyle.Render("  no profiles found"))
	}

	selectedName := m.selectedProfileName()
	if selectedName == "" {
		selectedName = "none"
	}
	sections := []string{
		titleStyle.Render("Profiles"),
		strings.Join(profileRows, "\n"),
		mutedStyle.Render(strings.Repeat("─", width)),
		titleStyle.Render("Databases: " + selectedName),
		m.databaseRows(),
		mutedStyle.Render(strings.Repeat("─", width)),
		titleStyle.Render("Add-ons: " + selectedName),
		m.addonRows(),
	}
	return strings.Join(sections, "\n")
}

func (m *Model) databaseRows() string {
	switch m.databasePhase {
	case phaseLoading:
		return mutedStyle.Render("  loading...")
	case phaseEmpty:
		return mutedStyle.Render("  no databases found")
	case phaseUnavailable:
		if len(m.databases) == 0 {
			return warningStyle.Render("  unavailable while stopped")
		}
		return warningStyle.Render("  unavailable; showing last snapshot") + "\n" + m.databaseSnapshotRows()
	case phaseError:
		if len(m.databases) == 0 {
			return errorStyle.Render("  refresh failed")
		}
		return errorStyle.Render("  refresh failed; showing last snapshot") + "\n" + m.databaseSnapshotRows()
	case phaseReady:
		return m.databaseSnapshotRows()
	default:
		return mutedStyle.Render("  not loaded")
	}
}

func (m *Model) databaseSnapshotRows() string {
	rows := make([]string, 0, len(m.databases))
	for index, database := range m.databases {
		rows = append(rows, m.row(databaseLabel(database), index == m.databaseIndex && m.focus == focusDatabases))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) addonRows() string {
	switch m.addonPhase {
	case phaseLoading:
		return mutedStyle.Render("  loading...")
	case phaseEmpty:
		return mutedStyle.Render("  no attached add-ons")
	case phaseError:
		return errorStyle.Render("  refresh failed")
	case phaseReady:
		rows := make([]string, 0, len(m.addons))
		for index, addon := range m.addons {
			rows = append(rows, m.row(addon.Name, index == m.addonIndex && m.focus == focusAddons))
		}
		return strings.Join(rows, "\n")
	default:
		return mutedStyle.Render("  not loaded")
	}
}

func (m *Model) rightView() string {
	switch m.focus {
	case focusDatabases:
		return m.databaseDetailView()
	case focusAddons:
		return m.addonDetailView()
	default:
		return m.profileDetailView()
	}
}

func (m *Model) profileDetailView() string {
	lines := []string{titleStyle.Render("Profile details")}
	if m.selectedProfileName() == "" {
		return strings.Join(append(lines, mutedStyle.Render("No profile selected")), "\n")
	}
	if m.profilePhase == phaseLoading {
		return strings.Join(append(lines, activeStyle.Render(m.selectedProfileName()), mutedStyle.Render("Loading profile details...")), "\n")
	}
	if m.profilePhase == phaseError {
		return strings.Join(append(lines, activeStyle.Render(m.selectedProfileName()), errorStyle.Render(errorText(m.profileDetailErr))), "\n")
	}
	detail := m.profileDetail
	lines = append(lines,
		activeStyle.Render(detail.Name),
		"docker state: "+profileState(detail.DockerState),
		"url: "+detail.URL,
		"odoo version: "+detail.OdooVersion,
		"attached add-ons: "+strings.Join(detail.AttachedAddons, ", "),
		"database prefix: "+detail.DatabasePrefix,
		"image: "+detail.Image,
		"filestore volume: "+detail.FilestoreVolume,
		"pending recreation: "+detail.PendingRecreation.String(),
	)
	return strings.Join(lines, "\n")
}

func (m *Model) databaseDetailView() string {
	lines := []string{titleStyle.Render("Database details")}
	if m.selectedProfileName() == "" {
		return strings.Join(append(lines, mutedStyle.Render("Select a profile")), "\n")
	}
	switch m.databasePhase {
	case phaseLoading:
		return strings.Join(append(lines, mutedStyle.Render("Loading databases...")), "\n")
	case phaseEmpty:
		return strings.Join(append(lines, mutedStyle.Render("No databases found")), "\n")
	case phaseUnavailable:
		lines = append(lines, warningStyle.Render("Databases are unavailable while the profile is stopped"))
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.databaseErr)))
	}
	database := m.selectedDatabase()
	if database == nil {
		return strings.Join(append(lines, mutedStyle.Render("No database selected")), "\n")
	}
	lines = append(lines, activeStyle.Render(databaseLabel(*database)))
	switch m.databaseInfoPhase {
	case phaseLoading:
		lines = append(lines, mutedStyle.Render("Loading database details..."))
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.databaseInfoErr)))
	case phaseUnavailable:
		lines = append(lines, warningStyle.Render("Database details are unavailable"))
		if m.databaseInfo.Physical != "" {
			lines = append(lines, mutedStyle.Render("Last known details"))
			lines = appendDatabaseInfo(lines, m.databaseInfo)
		}
	case phaseReady:
		lines = appendDatabaseInfo(lines, m.databaseInfo)
	}
	return strings.Join(lines, "\n")
}

func appendDatabaseInfo(lines []string, info odoo.DatabaseInfo) []string {
	return append(lines,
		"logical database: "+info.Logical,
		"physical database: "+info.Physical,
		"size: "+info.Size,
		"owner: "+info.Owner,
		"connection: "+connectionState(info.Available),
	)
}

func (m *Model) addonDetailView() string {
	lines := []string{titleStyle.Render("Add-on details")}
	if m.selectedProfileName() == "" {
		return strings.Join(append(lines, mutedStyle.Render("Select a profile")), "\n")
	}
	switch m.addonPhase {
	case phaseLoading:
		return strings.Join(append(lines, mutedStyle.Render("Loading attached add-ons...")), "\n")
	case phaseEmpty:
		return strings.Join(append(lines, mutedStyle.Render("No add-ons attached")), "\n")
	case phaseError:
		return strings.Join(append(lines, errorStyle.Render(errorText(m.addonErr))), "\n")
	}
	addon := m.selectedAddon()
	if addon == nil {
		return strings.Join(append(lines, mutedStyle.Render("No add-on selected")), "\n")
	}
	lines = append(lines, activeStyle.Render(addon.Name), "type: "+addon.Kind)
	if addon.Entry.Source != "" {
		lines = append(lines, "source: "+addon.Entry.Source)
	}
	if addon.Entry.WorktreeOf != "" {
		lines = append(lines, "worktree parent: "+addon.Entry.WorktreeOf)
	}
	if addon.Entry.Branch != "" {
		lines = append(lines, "branch: "+addon.Entry.Branch)
	}
	lines = append(lines, "path: "+addon.Path, "repository: "+repositoryState(*addon))
	if addon.Kind == "worktree" {
		lines = append(lines, "parent status: "+parentState(*addon))
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
	if m.watcherStarted && !m.watcherConnected {
		status += "  Docker events disconnected"
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

func databaseLabel(database odoo.Database) string {
	if database.Logical == database.Physical {
		return database.Physical
	}
	return database.Logical + "  (" + database.Physical + ")"
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

func connectionState(available bool) string {
	if available {
		return runningStyle.Render("available")
	}
	return warningStyle.Render("unavailable")
}

func repositoryState(addon addons.AddonStatus) string {
	if !addon.PathAvailable {
		return warningStyle.Render("unavailable")
	}
	if addon.Dirty {
		return warningStyle.Render("dirty")
	}
	return runningStyle.Render("clean")
}

func parentState(addon addons.AddonStatus) string {
	if addon.ParentAvailable {
		return runningStyle.Render("available")
	}
	return warningStyle.Render("unavailable")
}

func errorText(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

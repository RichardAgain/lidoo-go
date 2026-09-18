package tui

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
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
	focusLogs
	focusDatabases
	focusAddons
	focusInfo
	focusAreaCount
)

type modalMode uint8

const (
	modalNone modalMode = iota
	modalConfirmRemove
	modalRunVersion
	modalConfirmDrop
	modalInitDatabase
	modalBackupDatabase
	modalRestoreDatabase
	modalConfirmRestore
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
	logPhase               resourcePhase
	logOutput              string
	logErr                 error
	logRequest             uint64
	logResourceName        string
	logViewport            viewport.Model
	logViewportReady       bool
	databases              []odoo.Database
	databaseIndex          int
	addons                 []addons.AddonStatus
	addonIndex             int
	focus                  focusArea
	loading                bool
	err                    error
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
	watcherDisconnected    bool
	profileListOnlyRefresh bool
	pendingDatabaseRefresh bool
	modal                  modalMode
	versionInput           string
	taskID                 uint64
	pendingTask            *taskRequest
	taskContext            context.Context
	taskCancel             context.CancelFunc
	taskProgress           <-chan string
	taskProfileName        string
	taskDatabaseName       string
	taskDatabasePhysical   string
	taskName               string
	taskStatus             string
	taskOutput             string
	taskErr                error
	taskStarting           bool
	taskRunning            bool
	taskCancelRequested    bool
	interactivePreparing   bool
	formField              int
	initDatabaseName       string
	initModules            string
	backupDestination      string
	backupFormat           string
	backupFilestore        bool
	backupForce            bool
	restoreSource          string
	restoreCopy            bool
	restoreForce           bool
	restoreNeutralize      bool
	restoreJobs            string
}

var (
	titleStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D78F2"})
	activeStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#4338CA", Dark: "#A5B4FC"})
	errorStyle           = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#F97068"})
	mutedStyle           = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#667085", Dark: "#98A2B3"})
	runningStyle         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#027A48", Dark: "#6CE9A6"})
	warningStyle         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B54708", Dark: "#FEC84B"})
	borderStyle          = lipgloss.AdaptiveColor{Light: "#D0D5DD", Dark: "#475467"}
	activeBorderStyle    = lipgloss.AdaptiveColor{Light: "#7D78F2", Dark: "#A5B4FC"}
	inspectedBorderStyle = lipgloss.AdaptiveColor{Light: "#A7A7E8", Dark: "#7684B2"}
)

func NewModel(ctx context.Context, service *app.Service) *Model {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Model{
		ctx:           ctx,
		service:       service,
		loading:       true,
		width:         80,
		height:        24,
		profilePhase:  phaseLoading,
		logPhase:      phaseIdle,
		logViewport:   viewport.New(1, 1),
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
		if listOnly && m.selectedProfileName() == m.profileResourceName {
			return m, nil
		}
		return m, m.beginProfileResources()
	case ProfilesFailedMsg:
		m.profileListOnlyRefresh = false
		m.loading = false
		m.err = msg.Err
		return m, nil
	case ProfileWatcherStartedMsg:
		m.watchEvents = msg.Events
		m.watchErrors = msg.Errors
		m.watcherDisconnected = false
		return m, WaitProfileInvalidationCmd(m.watchEvents, m.watchErrors)
	case ProfileInvalidationMsg:
		return m, tea.Batch(WaitProfileInvalidationCmd(m.watchEvents, m.watchErrors), m.handleInvalidation(msg.Invalidation))
	case ProfileWatcherDisconnectedMsg:
		m.watcherDisconnected = true
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
		return m, nil
	case DatabasesLoadedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		selected := m.selectedDatabasePhysical()
		previousIndex := m.databaseIndex
		m.databases = append([]odoo.Database(nil), msg.Databases...)
		m.databaseIndex = previousIndex
		if len(m.databases) == 0 {
			m.databaseIndex = 0
		} else if m.databaseIndex >= len(m.databases) {
			m.databaseIndex = len(m.databases) - 1
		}
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
		} else {
			m.databasePhase = phaseUnavailable
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
		return m, nil
	case TaskStartedMsg:
		if !m.currentTask(msg.ID) || m.pendingTask == nil {
			return m, nil
		}
		request := *m.pendingTask
		m.pendingTask = nil
		m.taskStarting = false
		m.taskRunning = true
		m.taskStatus = "running"
		progress := make(chan string, 32)
		m.taskProgress = progress
		return m, tea.Batch(
			ExecuteProfileTaskCmd(m.taskContext, m.service, request, msg.ID, progress),
			WaitTaskProgressCmd(msg.ID, progress),
		)
	case TaskProgressMsg:
		if !m.currentTask(msg.ID) || !m.taskRunning {
			return m, nil
		}
		if msg.Text != "" {
			m.taskOutput = appendTaskOutput(m.taskOutput, msg.Text)
		}
		if m.taskProgress == nil {
			return m, nil
		}
		return m, WaitTaskProgressCmd(msg.ID, m.taskProgress)
	case TaskProgressDoneMsg:
		if !m.currentTask(msg.ID) {
			return m, nil
		}
		m.taskProgress = nil
		return m, nil
	case TaskCompletedMsg:
		if !m.currentTask(msg.ID) {
			return m, nil
		}
		m.finishTask("completed", msg.Output, nil)
		return m, m.refreshAfterTask(msg.ProfileName)
	case TaskFailedMsg:
		if !m.currentTask(msg.ID) {
			return m, nil
		}
		var required *app.ProfileVersionRequiredError
		if errors.As(msg.Err, &required) {
			m.finishTask("version required", msg.Output, nil)
			m.modal = modalRunVersion
			m.versionInput = ""
			return m, nil
		}
		m.finishTask("failed", msg.Output, msg.Err)
		if errors.Is(msg.Err, odoo.ErrDatabaseNotFound) && msg.ProfileName == m.selectedProfileName() {
			return m, m.refreshDatabasesAfterMissing()
		}
		return m, m.refreshAfterTask(msg.ProfileName)
	case TaskCancelledMsg:
		if !m.currentTask(msg.ID) {
			return m, nil
		}
		m.finishTask("cancelled", msg.Output, nil)
		return m, m.refreshAfterTask(msg.ProfileName)
	case ProfileLogsLoadedMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.logPhase = phaseReady
		m.logOutput = msg.Output
		m.logErr = nil
		return m, nil
	case ProfileLogsFailedMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.logPhase = phaseError
		m.logErr = msg.Err
		return m, nil
	case InteractiveCommandReadyMsg:
		m.interactivePreparing = false
		if msg.Command == nil {
			m.retainInteractiveOutput(interactiveTaskName(msg), "", errors.New("interactive command is unavailable"))
			return m, nil
		}
		return m, tea.ExecProcess(msg.Command, func(err error) tea.Msg {
			return InteractiveCommandFinishedMsg{
				Kind:         msg.Kind,
				ProfileName:  msg.ProfileName,
				DatabaseName: msg.DatabaseName,
				Err:          err,
			}
		})
	case InteractiveCommandFailedMsg:
		m.interactivePreparing = false
		m.retainInteractiveOutput(interactiveTaskName(msg), "", msg.Err)
		if errors.Is(msg.Err, odoo.ErrDatabaseNotFound) && msg.ProfileName == m.selectedProfileName() {
			return m, m.refreshDatabasesAfterMissing()
		}
		return m, nil
	case InteractiveCommandFinishedMsg:
		m.retainInteractiveOutput(interactiveTaskName(msg), "", msg.Err)
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m.updateModalKey(msg)
	}
	if m.taskStarting || m.taskRunning || m.interactivePreparing {
		switch msg.String() {
		case "c", "ctrl+c":
			m.cancelTask()
		}
		return m, nil
	}
	if m.focus == focusLogs {
		switch msg.String() {
		case "up", "k", "down", "j", "pgup", "pgdown", "f", "b", "u", "ctrl+u", "d", "ctrl+d", " ":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "right", "l":
		return m, m.moveFocus(1)
	case "shift+tab", "left", "h":
		return m, m.moveFocus(-1)
	case "up", "k":
		return m, m.moveFocusedSelection(-1)
	case "down", "j":
		return m, m.moveFocusedSelection(1)
	case "x":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			return m, m.queueTask(taskRequest{Kind: taskRun, ProfileName: m.selectedProfileName()})
		}
	case "r":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			return m, m.queueTask(taskRequest{Kind: taskRestart, ProfileName: m.selectedProfileName()})
		}
		return m, m.refreshProfiles()
	case "R":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			return m, m.queueTask(taskRequest{Kind: taskRecreate, ProfileName: m.selectedProfileName()})
		}
		if m.focus == focusDatabases {
			m.openRestoreForm()
		}
	case "s":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			return m, m.queueTask(taskRequest{Kind: taskStop, ProfileName: m.selectedProfileName()})
		}
	case "d":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			m.taskProfileName = m.selectedProfileName()
			m.modal = modalConfirmRemove
		} else if m.focus == focusDatabases {
			m.openDropConfirmation()
		}
	case "i":
		if m.focus == focusDatabases {
			m.openInitForm()
		}
	case "u":
		if m.focus == focusDatabases {
			return m, m.queueSelectedDatabaseTask(taskUpdate, false)
		}
	case "U":
		if m.focus == focusDatabases {
			return m, m.queueSelectedDatabaseTask(taskUpdate, true)
		}
	case "b":
		if m.focus == focusDatabases {
			m.openBackupForm()
		}
	case "p":
		if m.focus == focusDatabases {
			database := m.selectedDatabase()
			if database != nil {
				profileName := m.selectedProfileName()
				m.interactivePreparing = true
				m.taskName = "psql " + profileName + "/" + database.Logical
				m.taskStatus = "preparing"
				m.taskOutput = ""
				m.taskErr = nil
				m.err = nil
				return m, PrepareDatabaseShellCmd(m.ctx, m.service, profileName, database.Logical)
			}
		}
	case "ctrl+r":
		return m, m.refreshProfiles()
	case "?":
		m.showHelp = !m.showHelp
	}
	return m, nil
}

func (m *Model) prepareDatabaseAction() bool {
	if m.selectedProfileName() == "" || m.selectedDatabase() == nil {
		return false
	}
	database := m.selectedDatabase()
	m.taskProfileName = m.selectedProfileName()
	m.taskDatabaseName = database.Logical
	m.taskDatabasePhysical = database.Physical
	return true
}

func (m *Model) queueSelectedDatabaseTask(kind taskKind, updateAll bool) tea.Cmd {
	if !m.prepareDatabaseAction() {
		return nil
	}
	return m.queueTask(taskRequest{
		Kind:             kind,
		ProfileName:      m.taskProfileName,
		DatabaseName:     m.taskDatabaseName,
		DatabasePhysical: m.taskDatabasePhysical,
		UpdateAll:        updateAll,
	})
}

func (m *Model) openDropConfirmation() {
	if !m.prepareDatabaseAction() {
		return
	}
	m.modal = modalConfirmDrop
}

func (m *Model) openInitForm() {
	profileName := m.selectedProfileName()
	if profileName == "" {
		return
	}
	m.taskProfileName = profileName
	m.initDatabaseName = ""
	if database := m.selectedDatabase(); database != nil {
		m.initDatabaseName = database.Logical
	}
	m.taskDatabaseName = m.initDatabaseName
	m.initModules = "base"
	m.formField = 0
	m.modal = modalInitDatabase
}

func (m *Model) openBackupForm() {
	if !m.prepareDatabaseAction() {
		return
	}
	m.backupDestination = ""
	m.backupFormat = "zip"
	m.backupFilestore = true
	m.backupForce = false
	m.formField = 0
	m.modal = modalBackupDatabase
}

func (m *Model) openRestoreForm() {
	if !m.prepareDatabaseAction() {
		return
	}
	m.restoreSource = ""
	m.restoreCopy = true
	m.restoreForce = false
	m.restoreNeutralize = false
	m.restoreJobs = "1"
	m.formField = 0
	m.modal = modalRestoreDatabase
}

func (m *Model) refreshProfiles() tea.Cmd {
	m.profileListOnlyRefresh = false
	m.loading = true
	m.err = nil
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) updateModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalConfirmRemove:
		switch msg.String() {
		case "esc", "n", "N":
			m.modal = modalNone
		case "enter", "y", "Y":
			name := m.taskProfileName
			m.modal = modalNone
			return m, m.queueTask(taskRequest{Kind: taskRemove, ProfileName: name})
		}
	case modalConfirmDrop:
		switch msg.String() {
		case "esc", "n", "N":
			m.modal = modalNone
		case "enter", "y", "Y":
			m.modal = modalNone
			return m, m.queueTask(taskRequest{
				Kind:             taskDrop,
				ProfileName:      m.taskProfileName,
				DatabaseName:     m.taskDatabaseName,
				DatabasePhysical: m.taskDatabasePhysical,
				Yes:              true,
			})
		}
	case modalConfirmRestore:
		switch msg.String() {
		case "esc", "n", "N":
			m.modal = modalRestoreDatabase
		case "enter", "y", "Y":
			m.modal = modalNone
			return m, m.queueRestoreTask()
		}
	case modalRunVersion:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
		case "enter":
			version := strings.TrimSpace(m.versionInput)
			if version == "" {
				return m, nil
			}
			m.modal = modalNone
			return m, m.queueTask(taskRequest{Kind: taskRun, ProfileName: m.taskProfileName, Version: version})
		case "backspace", "ctrl+h":
			m.versionInput = removeLastRune(m.versionInput)
		case "ctrl+u":
			m.versionInput = ""
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.versionInput += value
			}
		}
	case modalInitDatabase, modalBackupDatabase, modalRestoreDatabase:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
		case "enter":
			return m, m.submitDatabaseForm()
		case "tab":
			m.moveFormField(1)
		case "shift+tab":
			m.moveFormField(-1)
		case "backspace", "ctrl+h":
			m.removeFormText()
		case "ctrl+u":
			m.clearFormText()
		case " ":
			if !m.toggleFormField() {
				m.appendFormText(" ")
			}
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.appendFormText(value)
			}
		}
	}
	return m, nil
}

func (m *Model) submitDatabaseForm() tea.Cmd {
	switch m.modal {
	case modalInitDatabase:
		databaseName := strings.TrimSpace(m.initDatabaseName)
		modules := strings.TrimSpace(m.initModules)
		if databaseName == "" || modules == "" {
			return nil
		}
		m.taskDatabaseName = databaseName
		m.modal = modalNone
		return m.queueTask(taskRequest{
			Kind:             taskInit,
			ProfileName:      m.taskProfileName,
			DatabaseName:     databaseName,
			DatabasePhysical: m.taskDatabasePhysical,
			Modules:          modules,
		})
	case modalBackupDatabase:
		m.modal = modalNone
		return m.queueTask(taskRequest{
			Kind:             taskBackup,
			ProfileName:      m.taskProfileName,
			DatabaseName:     m.taskDatabaseName,
			DatabasePhysical: m.taskDatabasePhysical,
			Destination:      strings.TrimSpace(m.backupDestination),
			Format:           m.backupFormat,
			Filestore:        m.backupFilestore,
			Force:            m.backupForce,
		})
	case modalRestoreDatabase:
		if strings.TrimSpace(m.restoreSource) == "" {
			return nil
		}
		if m.restoreForce || !m.restoreCopy {
			m.modal = modalConfirmRestore
			return nil
		}
		m.modal = modalNone
		return m.queueRestoreTask()
	}
	return nil
}

func (m *Model) queueRestoreTask() tea.Cmd {
	return m.queueTask(taskRequest{
		Kind:             taskRestore,
		ProfileName:      m.taskProfileName,
		DatabaseName:     m.taskDatabaseName,
		DatabasePhysical: m.taskDatabasePhysical,
		Source:           strings.TrimSpace(m.restoreSource),
		CopyDatabase:     m.restoreCopy,
		Force:            m.restoreForce,
		Neutralize:       m.restoreNeutralize,
		Jobs:             m.restoreJobs,
	})
}

func (m *Model) formFieldCount() int {
	switch m.modal {
	case modalInitDatabase:
		return 2
	case modalBackupDatabase:
		return 4
	case modalRestoreDatabase:
		return 5
	default:
		return 0
	}
}

func (m *Model) moveFormField(delta int) {
	count := m.formFieldCount()
	if count == 0 {
		return
	}
	m.formField = (m.formField + delta + count) % count
}

func (m *Model) toggleFormField() bool {
	switch m.modal {
	case modalBackupDatabase:
		switch m.formField {
		case 1:
			formats := []string{"zip", "dump", "folder"}
			for index, format := range formats {
				if m.backupFormat == format {
					m.backupFormat = formats[(index+1)%len(formats)]
					return true
				}
			}
			m.backupFormat = formats[0]
			return true
		case 2:
			m.backupFilestore = !m.backupFilestore
			return true
		case 3:
			m.backupForce = !m.backupForce
			return true
		}
	case modalRestoreDatabase:
		switch m.formField {
		case 1:
			m.restoreCopy = !m.restoreCopy
			return true
		case 2:
			m.restoreForce = !m.restoreForce
			return true
		case 3:
			m.restoreNeutralize = !m.restoreNeutralize
			return true
		}
	}
	return false
}

func (m *Model) formText() *string {
	switch m.modal {
	case modalInitDatabase:
		switch m.formField {
		case 0:
			return &m.initDatabaseName
		case 1:
			return &m.initModules
		}
	case modalBackupDatabase:
		switch m.formField {
		case 0:
			return &m.backupDestination
		}
	case modalRestoreDatabase:
		switch m.formField {
		case 0:
			return &m.restoreSource
		case 4:
			return &m.restoreJobs
		}
	}
	return nil
}

func (m *Model) appendFormText(value string) {
	input := m.formText()
	if input != nil {
		*input += value
	}
}

func (m *Model) removeFormText() {
	input := m.formText()
	if input != nil {
		*input = removeLastRune(*input)
	}
}

func (m *Model) clearFormText() {
	input := m.formText()
	if input != nil {
		*input = ""
	}
}

func removeLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func (m *Model) queueTask(request taskRequest) tea.Cmd {
	m.taskID++
	m.pendingTask = &request
	m.taskProfileName = request.ProfileName
	m.taskDatabaseName = request.DatabaseName
	m.taskDatabasePhysical = request.DatabasePhysical
	m.taskName = taskLabel(request.Kind) + " " + request.ProfileName
	if request.DatabaseName != "" {
		m.taskName += "/" + request.DatabaseName
	}
	m.taskStatus = "starting"
	m.taskOutput = ""
	m.taskErr = nil
	m.taskStarting = true
	m.taskRunning = false
	m.taskCancelRequested = false
	taskBaseContext := m.ctx
	if taskBaseContext == nil {
		taskBaseContext = context.Background()
	}
	m.taskContext, m.taskCancel = context.WithCancel(taskBaseContext)
	return StartTaskCmd(m.taskID)
}

func (m *Model) cancelTask() {
	if m.taskCancelRequested {
		return
	}
	m.taskCancelRequested = true
	m.taskStatus = "cancelling"
	if m.taskCancel != nil {
		m.taskCancel()
	}
}

func (m *Model) currentTask(id uint64) bool {
	return id == m.taskID && (m.taskStarting || m.taskRunning)
}

func (m *Model) finishTask(status, output string, taskErr error) {
	if m.taskCancel != nil {
		m.taskCancel()
	}
	m.taskCancel = nil
	m.taskContext = nil
	m.taskProgress = nil
	m.pendingTask = nil
	m.taskStarting = false
	m.taskRunning = false
	m.taskStatus = status
	m.taskCancelRequested = false
	m.taskOutput = strings.TrimSpace(output)
	m.taskErr = taskErr
}

func (m *Model) retainInteractiveOutput(name, output string, taskErr error) {
	m.taskName = name
	m.taskStatus = "completed"
	if taskErr != nil {
		m.taskStatus = "failed"
	}
	m.taskOutput = strings.TrimSpace(output)
	m.taskErr = taskErr
	m.interactivePreparing = false
}

func interactiveTaskName(message any) string {
	var kind interactiveKind
	var profileName, databaseName string
	switch message := message.(type) {
	case InteractiveCommandReadyMsg:
		kind, profileName, databaseName = message.Kind, message.ProfileName, message.DatabaseName
	case InteractiveCommandFailedMsg:
		kind, profileName, databaseName = message.Kind, message.ProfileName, message.DatabaseName
	case InteractiveCommandFinishedMsg:
		kind, profileName, databaseName = message.Kind, message.ProfileName, message.DatabaseName
	}
	if kind == interactiveDatabaseShell {
		return "psql " + profileName + "/" + databaseName
	}
	return "follow logs " + profileName
}

func (m *Model) refreshAfterTask(profileName string) tea.Cmd {
	if profileName != "" {
		m.taskProfileName = profileName
	}
	m.profileListOnlyRefresh = false
	m.loading = true
	m.err = nil
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) refreshDatabasesAfterMissing() tea.Cmd {
	profileName := m.selectedProfileName()
	if profileName == "" {
		return nil
	}
	m.databaseRequest++
	m.databasePhase = phaseLoading
	m.databaseErr = nil
	m.databaseInfoPhase = phaseIdle
	m.databaseInfoErr = nil
	return LoadDatabasesCmd(m.ctx, m.service, profileName, m.profileRequest)
}

func appendTaskOutput(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	return existing + "\n" + next
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
	profileName := m.selectedProfileName()
	previousLogProfile := m.logResourceName
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
	m.logRequest++
	if previousLogProfile != profileName {
		m.logPhase = phaseIdle
		m.logOutput = ""
		m.logErr = nil
		m.logResourceName = ""
		m.logViewportReady = false
	}
	m.addonPhase = phaseLoading
	m.addonErr = nil
	m.addons = nil
	m.addonIndex = 0

	m.profileResourceName = profileName
	if profileName == "" {
		m.profilePhase = phaseEmpty
		m.databasePhase = phaseEmpty
		m.databaseInfoPhase = phaseEmpty
		m.logPhase = phaseEmpty
		m.logOutput = ""
		m.logErr = nil
		m.logResourceName = ""
		m.logViewportReady = false
		m.addonPhase = phaseEmpty
		return nil
	}
	commands := []tea.Cmd{
		LoadProfileDetailCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadDatabasesCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadAddonsCmd(m.ctx, m.service, profileName, m.profileRequest),
	}
	if m.focus == focusLogs {
		commands = append(commands, m.beginLogLoad())
	}
	return tea.Batch(commands...)
}

func (m *Model) handleInvalidation(invalidation app.ProfileInvalidation) tea.Cmd {
	if invalidation.ProfileName == "" {
		return nil
	}
	if invalidation.ProfileName != m.selectedProfileName() {
		m.profileListOnlyRefresh = true
		return LoadProfilesCmd(m.ctx, m.service)
	}
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
	commands := []tea.Cmd{LoadProfileDetailCmd(m.ctx, m.service, invalidation.ProfileName, m.profileRequest)}
	if m.focus == focusLogs {
		commands = append(commands, m.beginLogLoad())
	}
	return tea.Batch(commands...)
}

func (m *Model) afterProfileDetailLoaded() tea.Cmd {
	if !m.pendingDatabaseRefresh {
		return nil
	}
	m.pendingDatabaseRefresh = false
	if !strings.EqualFold(m.profileDetail.DockerState, "running") {
		m.databasePhase = phaseUnavailable
		m.databaseInfoPhase = phaseUnavailable
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

func (m *Model) beginLogLoad() tea.Cmd {
	m.logRequest++
	m.logPhase = phaseLoading
	m.logErr = nil
	profileName := m.selectedProfileName()
	m.logResourceName = profileName
	if profileName == "" {
		m.logPhase = phaseEmpty
		return nil
	}
	return LoadProfileLogsCmd(m.ctx, m.service, profileName, m.logRequest)
}

func (m *Model) moveFocus(delta int) tea.Cmd {
	m.focus = focusArea((int(m.focus) + delta + int(focusAreaCount)) % int(focusAreaCount))
	if m.focus == focusLogs {
		return m.beginLogLoad()
	}
	return nil
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
	case focusInfo:
		// Profile information has no independently navigable rows.
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

func (m *Model) currentLogRequest(requestID uint64, profileName string) bool {
	return requestID == m.logRequest && profileName != "" && profileName == m.selectedProfileName() && profileName == m.logResourceName
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
	right := lipgloss.NewStyle().Width(rightWidth).Render(m.rightView(rightWidth))
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	footer := m.footerView()
	view := main + "\n" + mutedStyle.Render(strings.Repeat("─", m.width)) + "\n" + footer
	if m.modal != modalNone {
		view += "\n\n" + m.modalView()
	}
	return view
}

func (m *Model) leftView(width int) string {
	lines := []string{titleStyle.Render("Profiles")}
	if len(m.profiles) == 0 {
		if m.loading {
			lines = append(lines, mutedStyle.Render("loading..."))
		} else {
			lines = append(lines, mutedStyle.Render("no profiles found"))
		}
		return strings.Join(lines, "\n")
	}

	cards := make([]string, 0, len(m.profiles))
	for index, profile := range m.profiles {
		cards = append(cards, m.profileCard(width, profile, index == m.profileIndex))
	}
	return strings.Join(append(lines, strings.Join(cards, "\n\n")), "\n\n")
}

func (m *Model) profileCard(width int, profile docker.ProfileSummary, selected bool) string {
	cardWidth := width - 2
	if cardWidth < 1 {
		cardWidth = 1
	}
	border := borderStyle
	if selected {
		border = inspectedBorderStyle
		if m.focus == focusProfiles {
			border = activeBorderStyle
		}
	}
	row := m.row(profile.Name+"  "+profileState(profile.State), selected)
	return lipgloss.NewStyle().
		Width(cardWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Render(row)
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

func (m *Model) rightView(width int) string {
	var content string
	switch m.activeTab() {
	case focusLogs:
		content = m.logTabView(width)
	case focusDatabases:
		content = m.databaseTabView()
	case focusAddons:
		content = m.addonTabView()
	default:
		content = m.profileDetailView()
	}
	content = m.tabBar() + "\n\n" + content
	warnings := make([]string, 0, 3)
	if m.loading {
		warnings = append(warnings, mutedStyle.Render("Loading profiles..."))
	}
	if m.err != nil {
		warnings = append(warnings, errorStyle.Render(m.err.Error()))
	}
	if m.watcherDisconnected {
		warnings = append(warnings, warningStyle.Render("Docker event watcher disconnected"))
	}
	if task := m.taskView(); task != "" {
		content += "\n\n" + task
	}
	if len(warnings) == 0 {
		return content
	}
	return strings.Join(append(warnings, "", content), "\n")
}

func (m *Model) activeTab() focusArea {
	if m.focus == focusProfiles {
		return focusInfo
	}
	return m.focus
}

func (m *Model) tabBar() string {
	tabs := []struct {
		name  string
		focus focusArea
	}{
		{name: "Log", focus: focusLogs},
		{name: "Databases", focus: focusDatabases},
		{name: "Add-ons", focus: focusAddons},
		{name: "Info", focus: focusInfo},
	}
	labels := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		label := "  " + tab.name
		if m.focus != focusProfiles && m.activeTab() == tab.focus {
			label = activeStyle.Render("  " + tab.name)
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, "    ")
}

func (m *Model) logTabView(width int) string {
	profileName := m.selectedProfileName()
	if profileName == "" {
		profileName = "none"
	}
	return strings.Join([]string{
		titleStyle.Render("Logs: " + profileName),
		m.logContainer(width, m.logContent()),
	}, "\n\n")
}

func (m *Model) logContent() string {
	output := strings.TrimRight(m.logOutput, "\n")
	status := ""
	switch m.logPhase {
	case phaseLoading:
		status = mutedStyle.Render("refreshing profile logs...")
	case phaseReady:
		if output == "" {
			status = mutedStyle.Render("no log output")
		}
	case phaseError:
		status = errorStyle.Render("refresh failed: " + errorText(m.logErr))
	case phaseEmpty:
		status = mutedStyle.Render("select a profile to load logs")
	case phaseIdle:
		status = mutedStyle.Render("focus Log to load profile logs")
	}
	if status == "" {
		return output
	}
	if output == "" {
		return status
	}
	return status + "\n\n" + output
}

func (m *Model) logContainer(width int, content string) string {
	height := m.height - 10
	if height < 4 {
		height = 4
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderStyle).
		Padding(0, 1)
	wasBottom := !m.logViewportReady || m.logViewport.AtBottom()
	m.logViewport.Width = width
	m.logViewport.Height = height
	m.logViewport.Style = style
	m.logViewport.SetContent(content)
	if wasBottom {
		m.logViewport.GotoBottom()
	}
	m.logViewportReady = true
	return m.logViewport.View()
}

func (m *Model) databaseTabView() string {
	profileName := m.selectedProfileName()
	if profileName == "" {
		profileName = "none"
	}
	return strings.Join([]string{
		titleStyle.Render("Databases: " + profileName),
		m.databaseRows(),
		m.databaseDetailView(),
		m.databaseActionsView(),
	}, "\n\n")
}

func (m *Model) databaseActionsView() string {
	lines := []string{titleStyle.Render("Database actions")}
	if m.selectedProfileName() == "" {
		return strings.Join(append(lines, mutedStyle.Render("Select a profile to enable actions")), "\n")
	}
	if m.selectedDatabase() == nil {
		return strings.Join(append(lines,
			activeStyle.Render("i init new database"),
			mutedStyle.Render("Select a database for update, drop, backup, restore, or psql"),
		), "\n")
	}
	return strings.Join(append(lines,
		mutedStyle.Render("i init   u update   U update-all   d drop"),
		mutedStyle.Render("b backup   R restore   p psql"),
	), "\n")
}

func (m *Model) addonTabView() string {
	profileName := m.selectedProfileName()
	if profileName == "" {
		profileName = "none"
	}
	return strings.Join([]string{
		titleStyle.Render("Add-ons: " + profileName),
		m.addonRows(),
		m.addonDetailView(),
	}, "\n\n")
}

func (m *Model) taskView() string {
	if m.taskStatus == "" || m.taskName == "" {
		return ""
	}
	lines := []string{titleStyle.Render("Latest task"), m.taskName + "  " + taskStatus(m.taskStatus)}
	if m.taskErr != nil {
		lines = append(lines, errorStyle.Render(m.taskErr.Error()))
	}
	if m.taskOutput != "" {
		lines = append(lines, mutedStyle.Render("output:"), m.taskOutput)
	}
	return strings.Join(lines, "\n")
}

func taskStatus(status string) string {
	switch status {
	case "completed":
		return runningStyle.Render(status)
	case "failed":
		return errorStyle.Render(status)
	case "cancelled", "cancelling", "version required":
		return warningStyle.Render(status)
	default:
		return mutedStyle.Render(status)
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
	return activeStyle.Render("  " + value)
}

func (m *Model) footerView() string {
	keys := "tab/←/→ focus  ctrl+r refresh  ? help  q quit"
	if m.focus == focusProfiles && m.selectedProfileName() != "" {
		keys = "tab/←/→ focus  ↑/↓ profiles  " + profileActionHints() + "  ctrl+r refresh  ? help  q quit"
	}
	if m.focus == focusLogs && m.selectedProfileName() != "" {
		keys = "tab/←/→ focus  ↑/↓ scroll  ctrl+r refresh logs  ? help  q quit"
	}
	if m.focus == focusDatabases && m.selectedProfileName() != "" {
		keys = "tab/←/→ focus  ↑/↓ databases  i init new database"
		if m.selectedDatabase() != nil {
			keys += "  " + databaseActionHints()
		}
		keys += "  ctrl+r refresh  ? help  q quit"
	}
	if m.focus == focusAddons && len(m.addons) > 0 {
		keys = "tab/←/→ focus  ↑/↓ add-ons  ctrl+r refresh  ? help  q quit"
	}
	if m.showHelp {
		keys = "tab/←/→ focus  ↑/↓/j/k move  ctrl+r refresh  q quit  ? hide help"
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			keys = "tab/←/→ focus  ↑/↓/j/k profiles  " + profileActionHints() + "  ctrl+r refresh  ? hide help"
		}
		if m.focus == focusLogs && m.selectedProfileName() != "" {
			keys = "tab/←/→ focus  ↑/↓/j/k scroll  f/pgdn  b/pgup  ctrl+r refresh logs  ? hide help"
		}
		if m.focus == focusDatabases && m.selectedProfileName() != "" {
			keys = "tab/←/→ focus  ↑/↓/j/k databases  i init new database"
			if m.selectedDatabase() != nil {
				keys += "  " + databaseActionHints()
			}
			keys += "  ctrl+r refresh  ? hide help"
		}
		if m.focus == focusAddons && len(m.addons) > 0 {
			keys = "tab/←/→ focus  ↑/↓/j/k add-ons  ctrl+r refresh  ? hide help"
		}
	}
	if m.taskStarting || m.taskRunning {
		keys += "  c cancel"
	}
	if m.taskStatus != "" {
		keys += "  · " + m.taskName + ": " + m.taskStatus
	}
	return mutedStyle.Render(keys)
}

func profileActionHints() string {
	return "x start  r restart  R recreate  s stop  d remove"
}

func databaseActionHints() string {
	return "i init  u update  U update-all  d drop  b backup  R restore  p psql"
}

func (m *Model) modalView() string {
	var lines []string
	switch m.modal {
	case modalConfirmRemove:
		lines = []string{
			titleStyle.Render("Remove profile " + m.taskProfileName + "?"),
			"This removes the Docker container and profile workspace entry.",
			warningStyle.Render("It does not delete PostgreSQL data, databases, the filestore volume, or add-on checkouts."),
			"",
			mutedStyle.Render("enter/y confirm  n/esc cancel"),
		}
	case modalRunVersion:
		lines = []string{
			titleStyle.Render("Odoo version required"),
			"Profile: " + m.taskProfileName,
			"No stored Odoo version exists for this profile.",
			"version: " + activeStyle.Render(m.versionInput+"▏"),
			"",
			mutedStyle.Render("type a version  enter run  esc cancel"),
		}
	case modalConfirmDrop:
		lines = []string{
			titleStyle.Render("Drop database " + m.taskDatabaseName + "?"),
			"Profile: " + m.taskProfileName,
			"Physical database: " + m.taskDatabasePhysical,
			warningStyle.Render("This permanently deletes the database and its filestore."),
			"",
			mutedStyle.Render("enter/y confirm  n/esc cancel"),
		}
	case modalInitDatabase:
		lines = []string{
			titleStyle.Render("Initialize database"),
			"Profile: " + m.taskProfileName,
			m.formLine("database", m.initDatabaseName, 0, true),
			m.formLine("modules", m.initModules, 1, true),
			"",
			mutedStyle.Render("tab field  type  enter run  esc cancel"),
		}
	case modalBackupDatabase:
		destination := m.backupDestination
		if destination == "" {
			destination = "(default backups/<database>_...zip)"
		}
		lines = []string{
			titleStyle.Render("Backup database"),
			"Profile: " + m.taskProfileName,
			"Database: " + databaseLabel(odoo.Database{Logical: m.taskDatabaseName, Physical: m.taskDatabasePhysical}),
			m.formLine("destination", destination, 0, m.formField == 0),
			m.formLine("format", m.backupFormat, 1, false),
			m.formLine("filestore", yesNo(m.backupFilestore), 2, false),
			m.formLine("overwrite", yesNo(m.backupForce), 3, false),
			"",
			mutedStyle.Render("tab field  space toggle  enter run  esc cancel"),
		}
	case modalRestoreDatabase:
		source := m.restoreSource
		if source == "" {
			source = "(required)"
		}
		lines = []string{
			titleStyle.Render("Restore database"),
			"Profile: " + m.taskProfileName,
			"Database: " + databaseLabel(odoo.Database{Logical: m.taskDatabaseName, Physical: m.taskDatabasePhysical}),
			m.formLine("source", source, 0, m.formField == 0),
			m.formLine("mode", restoreMode(m.restoreCopy), 1, false),
			m.formLine("overwrite", yesNo(m.restoreForce), 2, false),
			m.formLine("neutralize", yesNo(m.restoreNeutralize), 3, false),
			m.formLine("jobs", m.restoreJobs, 4, m.formField == 4),
			"",
			mutedStyle.Render("tab field  space toggle  enter run  esc cancel"),
		}
	case modalConfirmRestore:
		lines = []string{
			titleStyle.Render("Confirm destructive restore"),
			"Database: " + m.taskDatabaseName,
			"Source: " + m.restoreSource,
			warningStyle.Render("Move or overwrite can replace existing database data."),
			"",
			mutedStyle.Render("enter/y confirm  n/esc back"),
		}
	}
	modalWidth := m.width - 4
	if modalWidth < 20 {
		modalWidth = m.width - 2
	}
	if modalWidth < 1 {
		modalWidth = 1
	}
	if modalWidth > 72 {
		modalWidth = 72
	}
	return lipgloss.NewStyle().Width(modalWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(activeBorderStyle).Render(strings.Join(lines, "\n"))
}

func (m *Model) formLine(label, value string, field int, textField bool) string {
	if textField && m.formField == field {
		value += "▏"
	}
	line := label + ": " + value
	if m.formField == field {
		return activeStyle.Render("  " + line)
	}
	return "  " + line
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func restoreMode(copyDatabase bool) string {
	if copyDatabase {
		return "copy"
	}
	return "move"
}

func (m *Model) smallView() string {
	lines := []string{titleStyle.Render("Lidoo"), mutedStyle.Render("Terminal too small for the full layout")}
	if m.loading {
		lines = append(lines, mutedStyle.Render("Loading profiles..."))
	}
	if m.err != nil {
		lines = append(lines, errorStyle.Render(m.err.Error()))
	}
	if len(m.profiles) > 0 {
		cards := make([]string, 0, len(m.profiles))
		for index, profile := range m.profiles {
			cards = append(cards, m.profileCard(m.width, profile, index == m.profileIndex))
		}
		lines = append(lines, "", titleStyle.Render("Profiles"), strings.Join(cards, "\n\n"))
	}
	if m.focus == focusDatabases && m.selectedProfileName() != "" {
		lines = append(lines, "", m.databaseTabView())
	}
	hints := profileActionHints()
	if m.focus == focusDatabases {
		hints = "i init new database"
		if m.selectedDatabase() != nil {
			hints = databaseActionHints()
		}
	}
	lines = append(lines, "", mutedStyle.Render(hints+"  ctrl+r refresh  ? help  q quit"))
	if task := m.taskView(); task != "" {
		lines = append(lines, "", task)
	}
	if m.showHelp {
		lines = append(lines, mutedStyle.Render("tab focus  arrows or j/k move  c cancel task"))
	}
	view := strings.Join(lines, "\n")
	if m.modal != modalNone {
		view += "\n\n" + m.modalView()
	}
	return view
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

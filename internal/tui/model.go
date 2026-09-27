package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/addons"
	"lidoo/internal/app"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/odoo"
	"lidoo/internal/profile"
)

type focusArea uint8

const (
	focusProfiles focusArea = iota
	focusDatabases
	focusAddons
	focusInfo
	focusAreaCount
)

type modalMode uint8

const (
	modalNone modalMode = iota
	modalTypedConfirm
	modalRunVersion
	modalInitDatabase
	modalBackupDatabase
	modalRestoreDatabase
	modalAddonMount
	modalCloneAddon
	modalWorktreeAddon
	modalConfirmWorktreeAddon
	modalRemoveAddon
	modalConfirmAddonRemove
	modalSelect
	modalCreateProfile
	modalProfileSettings
	modalConfigPattern
	modalConfigPassword
	modalAdminPassword
	modalFilePicker
)

type selectKind uint8

const (
	selectNone selectKind = iota
	selectRunProfileVersion
	selectCreateProfileVersion
	selectConfigDBFilterMode
	selectAddonWorktreeSource
	selectAddonWorktreeBranch
	selectRemoveAddon
	selectLogDatabase
)

// selectCustomValue is the sentinel value for "type a value instead of
// choosing one" entries in a select modal.
const selectCustomValue = "\x00custom"

type selectItem struct {
	Label string
	Value string
	Hint  string
}

type versionPurpose uint8

const (
	versionForRun versionPurpose = iota
	versionForCreate
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

const maxLogBufferSize = 64 * 1024

type profileLogBuffer struct {
	container         string
	containerSnapshot string
	containerLoaded   bool
	task              string
	entries           []logEntry
	dirty             bool
}

type Model struct {
	ctx                    context.Context
	service                *app.Service
	profiles               []docker.ProfileSummary
	profileIndex           int
	logPhase               resourcePhase
	profileLogs            map[string]*profileLogBuffer
	logErr                 error
	logRequest             uint64
	logResourceName        string
	logContext             context.Context
	logCancel              context.CancelFunc
	logProgress            <-chan profileLogStreamEvent
	logSnapshotPending     bool
	logPendingOutput       string
	logViewport            viewport.Model
	logViewportReady       bool
	logViewerOpen          bool
	logViewerProfile       string
	logViewerSource        logSourceFilter
	logViewerLevel         logLevelFilter
	logViewerSearch        string
	logViewerDatabase      string
	logDatabase            string
	logViewerTyping        bool
	logViewerFollow        bool
	logViewerViewport      viewport.Model
	logViewerReady         bool
	notice                 string
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
	addonChoiceRequest     uint64
	addonChoiceProfile     string
	addonChoicePhase       resourcePhase
	addonChoiceErr         error
	addonChoices           []addons.AddonStatus
	addonChoiceIndex       int
	addonChoiceSelected    map[string]bool
	addonMountAttach       bool
	addonMountRecreate     bool
	addonFormErr           error
	cloneAddonName         string
	cloneAddonURL          string
	cloneAddonDepth        string
	cloneAddonBranch       string
	worktreeAddonSource    string
	worktreeAddonName      string
	worktreeAddonBranch    string
	removeAddonName        string
	removeAddonForce       bool
	watchEvents            <-chan app.ProfileInvalidation
	watchErrors            <-chan error
	watcherDisconnected    bool
	profileListOnlyRefresh bool
	pendingDatabaseRefresh bool
	modal                  modalMode
	versionInput           string
	taskID                 uint64
	pendingTask            *taskRequest
	activeTaskKind         taskKind
	activitySpinner        spinner.Model
	taskContext            context.Context
	taskCancel             context.CancelFunc
	taskProgress           <-chan taskProgressEvent
	taskProfileName        string
	taskDatabaseName       string
	taskDatabasePhysical   string
	taskName               string
	taskStatus             string
	taskErr                error
	taskNotice             string
	taskOutput             string
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
	restoreDestination     string
	restoreErr             error
	restoreCopy            bool
	restoreForce           bool
	restoreNeutralize      bool
	restoreJobs            string
	selectKind             selectKind
	selectTitle            string
	selectItems            []selectItem
	selectIndex            int
	selectPhase            resourcePhase
	selectErr              error
	selectRequest          uint64
	selectSource           string
	versionPurpose         versionPurpose
	filePickerPath         string
	filePickerItems        []files.DirEntry
	filePickerIndex        int
	filePickerErr          error
	filePickerPhase        resourcePhase
	filePickerRequest      uint64
	createProfileVersion   string
	createProfileName      string
	createFormErr          error
	configProfileName      string
	configDBFilterMode     string
	configDBFilterPattern  string
	configAdminPasswd      string
	configFormErr          error
	profileSettingsIndex   int
	configPatternDraft     string
	configPasswordDraft    string
	adminPassword          string
	adminPasswordConfirm   string
	adminPasswordErr       error
	confirmAction          func() tea.Cmd
	confirmPrompt          string
	confirmExpected        string
	confirmInput           string
	confirmErr             error
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
		ctx:               ctx,
		service:           service,
		loading:           true,
		width:             80,
		height:            24,
		profilePhase:      phaseLoading,
		logPhase:          phaseIdle,
		profileLogs:       make(map[string]*profileLogBuffer),
		logViewport:       viewport.New(1, 1),
		logViewerViewport: viewport.New(1, 1),
		databasePhase:     phaseIdle,
		addonPhase:        phaseIdle,
		activitySpinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(activeStyle)),
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(LoadProfilesCmd(m.ctx, m.service), StartProfileWatcherCmd(m.ctx, m.service))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case spinner.TickMsg:
		if !m.taskActive() {
			return m, nil
		}
		var cmd tea.Cmd
		m.activitySpinner, cmd = m.activitySpinner.Update(msg)
		return m, cmd
	case ProfileURLOpenFailedMsg:
		m.err = msg.Err
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case ProfilesLoadedMsg:
		listOnly := m.profileListOnlyRefresh
		m.profileListOnlyRefresh = false
		previousSelection := m.selectedProfileName()
		m.setProfiles(msg.Profiles)
		if m.modal == modalAddonMount && m.selectedProfileName() != previousSelection {
			m.closeAddonMountChooser()
		}
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
		if m.focus == focusProfiles && !m.profileIsRunning() {
			m.setProfileLogsUnavailable(msg.ProfileName)
		}
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
		if m.logDatabase != "" {
			stillPresent := false
			for _, database := range m.databases {
				if database.Physical == m.logDatabase {
					stillPresent = true
					break
				}
			}
			if !stillPresent {
				m.logDatabase = ""
			}
		}
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
		if m.modal == modalAddonMount && !m.addonMountAttach && msg.ProfileName == m.addonChoiceProfile {
			m.addonChoices = append([]addons.AddonStatus(nil), m.addons...)
			m.addonChoiceIndex = 0
			m.addonChoiceSelected = make(map[string]bool, len(m.addonChoices))
			m.addonChoiceErr = nil
			m.addonChoicePhase = m.addonPhase
		}
		return m, nil
	case AddonsFailedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.addonPhase = phaseError
		m.addonErr = msg.Err
		if m.modal == modalAddonMount && !m.addonMountAttach && msg.ProfileName == m.addonChoiceProfile {
			m.addonChoicePhase = phaseError
			m.addonChoiceErr = msg.Err
		}
		return m, nil
	case AvailableAddonsLoadedMsg:
		if !m.currentAddonChoiceRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.addonChoices = append([]addons.AddonStatus(nil), msg.Addons...)
		m.addonChoiceSelected = make(map[string]bool, len(m.addonChoices))
		m.addonChoiceIndex = 0
		m.addonChoiceErr = nil
		if len(m.addonChoices) == 0 {
			m.addonChoicePhase = phaseEmpty
		} else {
			m.addonChoicePhase = phaseReady
		}
		return m, nil
	case AvailableAddonsFailedMsg:
		if !m.currentAddonChoiceRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.addonChoicePhase = phaseError
		m.addonChoiceErr = msg.Err
		return m, nil
	case VersionsLoadedMsg:
		if !m.currentSelectRequest(msg.RequestID) {
			return m, nil
		}
		switch m.selectKind {
		case selectRunProfileVersion, selectCreateProfileVersion:
			m.setVersionSelectItems(msg.Options)
			return m, nil
		default:
			return m, nil
		}
	case VersionsFailedMsg:
		if !m.currentSelectRequest(msg.RequestID) {
			return m, nil
		}
		if m.selectKind == selectRunProfileVersion || m.selectKind == selectCreateProfileVersion {
			// Even when discovery fails, let the operator type a version by hand.
			m.selectErr = msg.Err
			m.selectItems = []selectItem{{Label: "Custom version…", Value: selectCustomValue}}
			m.selectIndex = 0
			m.selectPhase = phaseReady
			return m, nil
		}
		m.selectPhase = phaseError
		m.selectErr = msg.Err
		return m, nil
	case ProfileConfigLoadedMsg:
		if !m.currentSelectRequest(msg.RequestID) || m.selectKind != selectConfigDBFilterMode {
			return m, nil
		}
		m.configProfileName = msg.ProfileName
		m.applyLoadedProfileConfig(msg.Config)
		return m, nil
	case ProfileConfigFailedMsg:
		if !m.currentSelectRequest(msg.RequestID) || m.selectKind != selectConfigDBFilterMode {
			return m, nil
		}
		m.selectPhase = phaseError
		m.selectErr = msg.Err
		return m, nil
	case AllAddonsLoadedMsg:
		if !m.currentSelectRequest(msg.RequestID) {
			return m, nil
		}
		switch m.selectKind {
		case selectAddonWorktreeSource:
			m.setAddonSelectItems(msg.Addons, true)
			return m, nil
		case selectRemoveAddon:
			m.setAddonSelectItems(msg.Addons, false)
			return m, nil
		default:
			return m, nil
		}
	case AllAddonsFailedMsg:
		if !m.currentSelectRequest(msg.RequestID) {
			return m, nil
		}
		m.selectPhase = phaseError
		m.selectErr = msg.Err
		return m, nil
	case AddonBranchesLoadedMsg:
		if !m.currentSelectRequest(msg.RequestID) || m.selectKind != selectAddonWorktreeBranch {
			return m, nil
		}
		m.selectSource = msg.Source
		m.setBranchSelectItems(msg.Branches)
		return m, nil
	case AddonBranchesFailedMsg:
		if !m.currentSelectRequest(msg.RequestID) || m.selectKind != selectAddonWorktreeBranch {
			return m, nil
		}
		m.selectPhase = phaseError
		m.selectErr = msg.Err
		return m, nil
	case DirectoryLoadedMsg:
		if msg.RequestID != m.filePickerRequest || m.modal != modalFilePicker {
			return m, nil
		}
		m.filePickerPath = msg.Path
		items := make([]files.DirEntry, 0, len(msg.Entries)+1)
		if parent := filepath.Dir(msg.Path); parent != msg.Path {
			items = append(items, files.DirEntry{Name: "..", Path: parent, Dir: true})
		}
		items = append(items, msg.Entries...)
		m.filePickerItems = items
		m.filePickerIndex = 0
		m.filePickerErr = nil
		if len(items) == 0 {
			m.filePickerPhase = phaseEmpty
		} else {
			m.filePickerPhase = phaseReady
		}
		return m, nil
	case DirectoryFailedMsg:
		if msg.RequestID != m.filePickerRequest || m.modal != modalFilePicker {
			return m, nil
		}
		m.filePickerPhase = phaseError
		m.filePickerErr = msg.Err
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
		progress := make(chan taskProgressEvent, 32)
		m.taskProgress = progress
		return m, tea.Batch(
			ExecuteProfileTaskCmd(m.taskContext, m.service, request, msg.ID, progress),
			WaitTaskProgressCmd(msg.ID, progress),
		)
	case TaskProgressMsg:
		if !m.currentTask(msg.ID) || !m.taskRunning || msg.ProfileName != m.taskProfileName {
			return m, nil
		}
		if msg.Text != "" {
			// Always keep the latest task output for the "Latest task" section,
			// and the per-profile buffer for the viewer's tasks tab.
			m.taskOutput = appendBoundedOutput(m.taskOutput, msg.Text)
			if msg.ProfileName != "" {
				m.appendTaskLog(msg.ProfileName, msg.Text)
			}
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
		m.finishTask("completed", nil)
		return m, m.refreshAfterTask(msg.ProfileName)
	case TaskFailedMsg:
		if !m.currentTask(msg.ID) {
			return m, nil
		}
		var required *app.ProfileVersionRequiredError
		if errors.As(msg.Err, &required) {
			m.finishTask("version required", nil)
			return m, m.openRunVersionSelect()
		}
		m.finishTask("failed", msg.Err)
		if errors.Is(msg.Err, odoo.ErrDatabaseNotFound) && msg.ProfileName == m.selectedProfileName() {
			return m, m.refreshDatabasesAfterMissing()
		}
		return m, m.refreshAfterTask(msg.ProfileName)
	case TaskCancelledMsg:
		if !m.currentTask(msg.ID) {
			return m, nil
		}
		m.finishTask("cancelled", nil)
		return m, m.refreshAfterTask(msg.ProfileName)
	case ProfileLogsLoadedMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.mergeContainerLogs(msg.ProfileName, msg.Output)
		m.flushPendingContainerLogs(msg.ProfileName)
		m.logSnapshotPending = false
		m.logPhase = phaseReady
		m.logErr = nil
		return m, nil
	case ProfileLogsFailedMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.flushPendingContainerLogs(msg.ProfileName)
		m.logSnapshotPending = false
		m.logPhase = phaseError
		m.logErr = msg.Err
		return m, nil
	case ProfileLogStreamMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) || msg.Text == "" {
			return m, nil
		}
		if m.logSnapshotPending {
			m.logPendingOutput = appendBoundedOutput(m.logPendingOutput, msg.Text)
		} else {
			m.appendContainerLog(msg.ProfileName, msg.Text)
		}
		if m.logProgress == nil {
			return m, nil
		}
		return m, WaitProfileLogStreamCmd(msg.ProfileName, msg.RequestID, m.logProgress)
	case ProfileLogStreamFailedMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.flushPendingContainerLogs(msg.ProfileName)
		m.logSnapshotPending = false
		m.logProgress = nil
		m.logPhase = phaseError
		m.logErr = msg.Err
		return m, nil
	case ProfileLogStreamDoneMsg:
		if !m.currentLogRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.flushPendingContainerLogs(msg.ProfileName)
		m.logSnapshotPending = false
		m.logProgress = nil
		return m, nil
	case InteractiveCommandReadyMsg:
		m.interactivePreparing = false
		if msg.Command == nil {
			m.retainInteractiveOutput(interactiveTaskName(msg), errors.New("interactive command is unavailable"))
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
		m.retainInteractiveOutput(interactiveTaskName(msg), msg.Err)
		if errors.Is(msg.Err, odoo.ErrDatabaseNotFound) && msg.ProfileName == m.selectedProfileName() {
			return m, m.refreshDatabasesAfterMissing()
		}
		return m, nil
	case InteractiveCommandFinishedMsg:
		m.retainInteractiveOutput(interactiveTaskName(msg), msg.Err)
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits, even while a task is running or a modal/viewer is
	// open. Cancelling a running task is a deliberate action on "c".
	if msg.String() == "ctrl+c" {
		m.cancelTask()
		return m, tea.Quit
	}
	if m.modal != modalNone {
		return m.updateModalKey(msg)
	}
	if m.logViewerOpen {
		return m.updateLogViewerKey(msg)
	}
	m.notice = ""
	if m.interactivePreparing {
		if msg.String() == "c" {
			m.cancelTask()
		}
		return m, nil
	}
	if m.taskActive() {
		if msg.String() == "c" {
			m.cancelTask()
			return m, nil
		}
	}
	if m.focus == focusProfiles {
		switch msg.String() {
		case "pgup", "pgdown", "f", "b", "ctrl+u", "ctrl+d":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab", "right", "l":
		return m, m.moveFocus(1)
	case "shift+tab", "left", "h":
		return m, m.moveFocus(-1)
	case "up", "k":
		return m, m.moveFocusedSelection(-1)
	case "down", "j":
		return m, m.moveFocusedSelection(1)
	case "enter":
		if m.focus == focusProfiles {
			profile := m.selectedProfile()
			if profile != nil {
				return m, OpenProfileURLCmd(profile.URL)
			}
		}
	case "x":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			if m.profileIsRunning() {
				m.notice = "profile " + m.selectedProfileName() + " is already running"
				return m, nil
			}
			return m, m.queueTask(taskRequest{Kind: taskRun, ProfileName: m.selectedProfileName(), Wait: true})
		} else if m.focus == focusAddons {
			return m, m.startRemoveAddon()
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
			return m, m.startRestorePicker()
		}
	case "s":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			if !m.profileIsRunning() {
				m.notice = "profile " + m.selectedProfileName() + " is not running"
				return m, nil
			}
			return m, m.queueTask(taskRequest{Kind: taskStop, ProfileName: m.selectedProfileName()})
		}
	case " ":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			return m, m.toggleSelectedProfile()
		}
	case "L":
		if m.selectedProfileName() != "" {
			if m.focus == focusDatabases {
				if database := m.selectedDatabase(); database != nil {
					m.logDatabase = database.Physical
				}
			}
			m.openLogViewer()
		}
	case "D":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			return m, m.openLogDatabaseSelect()
		}
	case "d":
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			if m.rejectMutatingAction() {
				return m, nil
			}
			name := m.selectedProfileName()
			m.taskProfileName = name
			m.openTypedConfirm(
				"This removes the Docker container and profile workspace entry. PostgreSQL data, the filestore volume, and add-on checkouts are kept.",
				name,
				func() tea.Cmd {
					return m.queueTask(taskRequest{Kind: taskRemove, ProfileName: name})
				},
			)
		} else if m.focus == focusDatabases {
			m.openDropConfirmation()
		} else if m.focus == focusAddons {
			return m, m.openAddonMountChooser(false)
		}
	case "a", "A":
		if m.focus == focusAddons {
			return m, m.openAddonMountChooser(true)
		}
		if m.focus == focusDatabases {
			m.openAdminPasswordForm()
		}
	case "c":
		if m.focus == focusProfiles {
			return m, m.startCreateProfile()
		}
		if m.focus == focusAddons {
			m.openCloneAddonForm()
		}
	case "w":
		if m.focus == focusAddons {
			return m, m.startWorktreeAddon()
		}
	case "n":
		if m.focus == focusProfiles {
			return m, m.startCreateProfile()
		}
	case "e":
		if m.focus == focusProfiles {
			return m, m.openProfileConfig()
		}
	case "f":
		if m.focus == focusAddons {
			return m, m.queueSelectedAddonTask(taskFetchAddon)
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
			if m.rejectMutatingAction() {
				return m, nil
			}
			database := m.selectedDatabase()
			if database != nil {
				profileName := m.selectedProfileName()
				m.interactivePreparing = true
				m.taskName = "psql " + profileName + "/" + database.Logical
				m.taskStatus = "preparing"
				m.taskErr = nil
				m.err = nil
				return m, PrepareDatabaseShellCmd(m.ctx, m.service, profileName, database.Logical)
			}
		} else if m.focus == focusAddons {
			return m, m.queueSelectedAddonTask(taskPullAddon)
		}
	case "ctrl+r":
		return m, m.refreshProfiles()
	case "?":
		m.showHelp = !m.showHelp
	}
	return m, nil
}

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m, nil
	}
	if m.logViewerOpen {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp {
			m.logViewerFollow = false
		}
		var cmd tea.Cmd
		m.logViewerViewport, cmd = m.logViewerViewport.Update(msg)
		return m, cmd
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m, m.handleLeftClick(msg.X, msg.Y)
	}
	if m.focus == focusProfiles {
		var cmd tea.Cmd
		m.logViewport, cmd = m.logViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleLeftClick(x, y int) tea.Cmd {
	leftWidth := m.width / 3
	if leftWidth < 24 {
		leftWidth = 24
	}
	if leftWidth > 40 {
		leftWidth = 40
	}

	if x < leftWidth {
		index := profileIndexAtRow(y, len(m.profiles))
		if index < 0 {
			return nil
		}
		if index != m.profileIndex {
			m.focus = focusProfiles
			m.profileIndex = index
			m.profileListOnlyRefresh = false
			return m.beginProfileResources()
		}
		return m.setFocus(focusProfiles)
	}

	reached := x - (leftWidth + 2)
	header := m.rightViewHeaderLines()
	if y == header {
		if reached < 14 {
			return m.setFocus(focusDatabases)
		}
		if reached < 26 {
			return m.setFocus(focusAddons)
		}
		return m.setFocus(focusInfo)
	}

	row := y - (header + 4)
	switch m.focus {
	case focusDatabases:
		if row >= 0 && row < len(m.databases) && row != m.databaseIndex {
			m.databaseIndex = row
			return m.beginDatabaseInfoLoad()
		}
	case focusAddons:
		if row >= 0 && row < len(m.addons) {
			m.addonIndex = row
		}
	}
	return nil
}

// rightViewHeaderLines counts the warning lines rightView prepends before the
// tab bar (plus the blank separator when any warning is present).
func (m *Model) rightViewHeaderLines() int {
	count := 0
	if m.loading {
		count++
	}
	if m.err != nil {
		count++
	}
	if m.notice != "" {
		count++
	}
	if m.watcherDisconnected {
		count++
	}
	if count == 0 {
		return 0
	}
	return count + 1
}

// profileIndexAtRow maps a click row in the left panel to a profile index. Each
// card is five lines tall plus one blank separator, under a title and blank.
func profileIndexAtRow(y, count int) int {
	if count == 0 {
		return -1
	}
	if y < 2 {
		return 0
	}
	index := (y - 2) / 6
	if index >= count {
		index = count - 1
	}
	return index
}

func (m *Model) setFocus(area focusArea) tea.Cmd {
	if m.focus == area {
		return nil
	}
	previous := m.focus
	m.focus = area
	if area == focusProfiles {
		return m.beginLogLoad()
	}
	if previous == focusProfiles {
		m.stopLogStream()
		m.logRequest++
	}
	return nil
}

func (m *Model) toggleSelectedProfile() tea.Cmd {
	name := m.selectedProfileName()
	if name == "" || m.rejectMutatingAction() {
		return nil
	}
	if m.profileIsRunning() {
		return m.queueTask(taskRequest{Kind: taskStop, ProfileName: name})
	}
	return m.queueTask(taskRequest{Kind: taskRun, ProfileName: name, Wait: true})
}

func (m *Model) openTypedConfirm(description, expected string, action func() tea.Cmd) {
	m.confirmPrompt = description
	m.confirmExpected = expected
	m.confirmInput = ""
	m.confirmErr = nil
	m.confirmAction = action
	m.formField = 0
	m.modal = modalTypedConfirm
}

func (m *Model) submitTypedConfirm() tea.Cmd {
	if m.confirmInput != m.confirmExpected {
		m.confirmErr = errors.New("type the name exactly to confirm")
		return nil
	}
	action := m.confirmAction
	m.modal = modalNone
	m.confirmAction = nil
	m.confirmInput = ""
	m.confirmErr = nil
	if action == nil {
		return nil
	}
	return action()
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
	if !m.prepareDatabaseAction() || m.rejectMutatingAction() {
		return
	}
	name := m.taskDatabaseName
	m.openTypedConfirm(
		"This permanently deletes the database "+databaseLabel(odoo.Database{Logical: m.taskDatabaseName, Physical: m.taskDatabasePhysical})+" and its filestore.",
		name,
		func() tea.Cmd {
			return m.queueTask(taskRequest{
				Kind:             taskDrop,
				ProfileName:      m.taskProfileName,
				DatabaseName:     m.taskDatabaseName,
				DatabasePhysical: m.taskDatabasePhysical,
				Yes:              true,
			})
		},
	)
}

func (m *Model) openInitForm() {
	profileName := m.selectedProfileName()
	if profileName == "" || m.rejectMutatingAction() {
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
	if !m.prepareDatabaseAction() || m.rejectMutatingAction() {
		return
	}
	m.backupDestination = ""
	m.backupFormat = "zip"
	m.backupFilestore = true
	m.backupForce = false
	m.formField = 0
	m.modal = modalBackupDatabase
}

func (m *Model) openRestoreForm(source string) {
	if !m.prepareDatabaseAction() || m.rejectMutatingAction() {
		return
	}
	m.restoreSource = source
	m.restoreDestination = m.defaultRestoreDestination(source)
	m.restoreErr = nil
	m.restoreCopy = true
	m.restoreForce = false
	m.restoreNeutralize = false
	m.restoreJobs = "1"
	m.formField = 0
	m.modal = modalRestoreDatabase
}

// defaultRestoreDestination derives a fresh, non-colliding database name from
// the dump file name so restoring never silently targets an existing database.
func (m *Model) defaultRestoreDestination(source string) string {
	return odoo.DeriveRestoreDestination(source, m.databases)
}

func (m *Model) restoreDestinationExists(name string) bool {
	for _, database := range m.databases {
		if database.Logical == name || database.Physical == name {
			return true
		}
	}
	return false
}

func (m *Model) startRestorePicker() tea.Cmd {
	if !m.prepareDatabaseAction() || m.rejectMutatingAction() {
		return nil
	}
	dir, err := os.Getwd()
	if err != nil || dir == "" {
		dir = "."
	}
	return m.openFilePicker(dir)
}

func (m *Model) openFilePicker(path string) tea.Cmd {
	m.filePickerRequest++
	m.filePickerPath = path
	m.filePickerItems = nil
	m.filePickerIndex = 0
	m.filePickerErr = nil
	m.filePickerPhase = phaseLoading
	m.modal = modalFilePicker
	return LoadDirectoryCmd(path, m.filePickerRequest)
}

func (m *Model) chooseFilePickerEntry() tea.Cmd {
	if m.filePickerPhase != phaseReady || m.filePickerIndex < 0 || m.filePickerIndex >= len(m.filePickerItems) {
		return nil
	}
	entry := m.filePickerItems[m.filePickerIndex]
	if entry.Dir {
		return m.openFilePicker(entry.Path)
	}
	source := entry.Path
	m.modal = modalNone
	m.openRestoreForm(source)
	return nil
}

func (m *Model) filePickerParent() tea.Cmd {
	parent := filepath.Dir(m.filePickerPath)
	if parent == m.filePickerPath {
		return nil
	}
	return m.openFilePicker(parent)
}

func (m *Model) openAddonMountChooser(attach bool) tea.Cmd {
	profileName := m.selectedProfileName()
	if profileName == "" || m.rejectMutatingAction() {
		return nil
	}
	m.addonChoiceRequest++
	m.addonChoiceProfile = profileName
	m.addonMountAttach = attach
	m.addonMountRecreate = false
	m.addonChoiceErr = nil
	m.addonChoiceIndex = 0
	m.addonChoiceSelected = make(map[string]bool)
	m.addonChoices = nil
	m.modal = modalAddonMount
	if attach {
		m.addonChoicePhase = phaseLoading
		return LoadAvailableAddonsCmd(m.ctx, m.service, profileName, m.addonChoiceRequest)
	}

	m.addonChoices = append([]addons.AddonStatus(nil), m.addons...)
	m.addonChoicePhase = m.addonPhase
	if m.addonChoicePhase == phaseIdle {
		m.addonChoicePhase = phaseLoading
		return LoadAddonsCmd(m.ctx, m.service, profileName, m.profileRequest)
	}
	if m.addonChoicePhase == phaseError {
		m.addonChoiceErr = m.addonErr
	}
	return nil
}

func (m *Model) openCloneAddonForm() {
	if m.rejectMutatingAction() {
		return
	}
	m.cloneAddonName = ""
	m.cloneAddonURL = ""
	m.cloneAddonDepth = "0"
	m.cloneAddonBranch = ""
	m.addonFormErr = nil
	m.formField = 0
	m.modal = modalCloneAddon
}

func (m *Model) startWorktreeAddon() tea.Cmd {
	if m.rejectMutatingAction() {
		return nil
	}
	m.selectRequest++
	m.selectKind = selectAddonWorktreeSource
	m.selectTitle = "Worktree source add-on"
	m.selectItems = nil
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.modal = modalSelect
	return LoadAllAddonsCmd(m.ctx, m.service, m.selectRequest)
}

func (m *Model) openWorktreeForm(source, branch string) {
	m.worktreeAddonSource = source
	m.worktreeAddonName = ""
	m.worktreeAddonBranch = branch
	m.addonFormErr = nil
	m.formField = 0
	m.modal = modalWorktreeAddon
}

func (m *Model) openWorktreeBranchSelect(source string) tea.Cmd {
	m.selectRequest++
	m.selectSource = source
	m.selectKind = selectAddonWorktreeBranch
	m.selectTitle = "Branch · " + source
	m.selectItems = nil
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.modal = modalSelect
	return LoadAddonBranchesCmd(m.ctx, m.service, source, m.selectRequest)
}

func (m *Model) startRemoveAddon() tea.Cmd {
	if m.rejectMutatingAction() {
		return nil
	}
	m.selectRequest++
	m.selectKind = selectRemoveAddon
	m.selectTitle = "Remove add-on checkout"
	m.selectItems = nil
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.modal = modalSelect
	return LoadAllAddonsCmd(m.ctx, m.service, m.selectRequest)
}

func (m *Model) openRemoveAddonForm(name string) {
	m.removeAddonName = name
	m.removeAddonForce = false
	m.addonFormErr = nil
	m.formField = 0
	m.modal = modalRemoveAddon
}

func (m *Model) queueSelectedAddonTask(kind taskKind) tea.Cmd {
	addon := m.selectedAddon()
	if addon == nil || m.selectedProfileName() == "" {
		return nil
	}
	return m.queueTask(taskRequest{
		Kind:        kind,
		ProfileName: m.selectedProfileName(),
		AddonName:   addon.Name,
	})
}

func (m *Model) closeAddonMountChooser() {
	m.addonChoiceRequest++
	m.modal = modalNone
}

func (m *Model) submitAddonMount() tea.Cmd {
	if m.addonChoicePhase != phaseReady {
		return nil
	}
	selected := make([]string, 0, len(m.addonChoiceSelected))
	for _, addon := range m.addonChoices {
		if m.addonChoiceSelected[addon.Name] {
			selected = append(selected, addon.Name)
		}
	}
	if len(selected) == 0 {
		m.addonChoiceErr = errors.New("select at least one add-on")
		return nil
	}
	request := taskRequest{
		Kind:        taskAttachAddons,
		ProfileName: m.addonChoiceProfile,
		AddonNames:  selected,
		Recreate:    m.addonMountRecreate,
	}
	if !m.addonMountAttach {
		request.Kind = taskDetachAddons
	}
	m.closeAddonMountChooser()
	return m.queueTask(request)
}

func (m *Model) currentSelectRequest(requestID uint64) bool {
	return requestID == m.selectRequest && m.modal == modalSelect
}

func (m *Model) closeSelect() {
	m.selectRequest++
	m.selectKind = selectNone
	m.selectItems = nil
	m.modal = modalNone
}

func (m *Model) setVersionSelectItems(options []docker.VersionOption) {
	items := make([]selectItem, 0, len(options)+1)
	for _, option := range options {
		hints := make([]string, 0, 2)
		if option.Dockerfile {
			hints = append(hints, "dockerfile")
		} else {
			hints = append(hints, "no dockerfile")
		}
		if option.Image {
			hints = append(hints, "image built")
		}
		items = append(items, selectItem{Label: option.Version, Value: option.Version, Hint: strings.Join(hints, " · ")})
	}
	items = append(items, selectItem{Label: "Custom version…", Value: selectCustomValue})
	m.selectItems = items
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseReady
}

func (m *Model) setAddonSelectItems(statuses []addons.AddonStatus, sourcesOnly bool) {
	items := make([]selectItem, 0, len(statuses))
	for _, status := range statuses {
		if sourcesOnly && status.Kind == "worktree" {
			continue
		}
		hint := status.Kind
		if status.Entry.Branch != "" {
			hint = status.Kind + " · " + status.Entry.Branch
		}
		if !status.PathAvailable {
			hint = strings.TrimSpace(hint + " · path unavailable")
		}
		items = append(items, selectItem{Label: status.Name, Value: status.Name, Hint: hint})
	}
	m.selectItems = items
	m.selectIndex = 0
	m.selectErr = nil
	if len(items) == 0 {
		m.selectPhase = phaseEmpty
	} else {
		m.selectPhase = phaseReady
	}
}

func (m *Model) setBranchSelectItems(branches []string) {
	items := make([]selectItem, 0, len(branches)+1)
	for _, branch := range branches {
		items = append(items, selectItem{Label: branch, Value: branch})
	}
	items = append(items, selectItem{Label: "New branch…", Value: selectCustomValue})
	m.selectItems = items
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseReady
}

func (m *Model) applySelectChoice() tea.Cmd {
	if m.selectPhase != phaseReady || m.selectIndex < 0 || m.selectIndex >= len(m.selectItems) {
		return nil
	}
	choice := m.selectItems[m.selectIndex]
	kind := m.selectKind
	m.selectRequest++
	m.selectKind = selectNone
	m.selectItems = nil
	switch kind {
	case selectRunProfileVersion:
		if choice.Value == selectCustomValue {
			m.modal = modalRunVersion
			m.versionPurpose = versionForRun
			m.versionInput = ""
			return nil
		}
		m.modal = modalNone
		return m.queueTask(taskRequest{Kind: taskRun, ProfileName: m.taskProfileName, Version: choice.Value, Wait: true})
	case selectCreateProfileVersion:
		if choice.Value == selectCustomValue {
			m.modal = modalRunVersion
			m.versionPurpose = versionForCreate
			m.versionInput = ""
			return nil
		}
		m.openCreateProfileForm(choice.Value)
		return nil
	case selectConfigDBFilterMode:
		return m.applySelectedConfigMode(choice.Value)
	case selectAddonWorktreeSource:
		m.modal = modalNone
		return m.openWorktreeBranchSelect(choice.Value)
	case selectAddonWorktreeBranch:
		m.modal = modalNone
		branch := choice.Value
		if branch == selectCustomValue {
			branch = ""
		}
		m.openWorktreeForm(m.selectSource, branch)
		return nil
	case selectRemoveAddon:
		m.modal = modalNone
		m.openRemoveAddonForm(choice.Value)
		return nil
	case selectLogDatabase:
		m.logDatabase = choice.Value
		m.logViewerDatabase = choice.Value
		m.modal = modalNone
		return nil
	default:
		m.modal = modalNone
		return nil
	}
}

func (m *Model) startCreateProfile() tea.Cmd {
	if m.rejectMutatingAction() {
		return nil
	}
	m.selectRequest++
	m.selectKind = selectCreateProfileVersion
	m.selectTitle = "New profile · Odoo version"
	m.selectItems = nil
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.modal = modalSelect
	return LoadVersionsCmd(m.ctx, m.service, m.selectRequest)
}

func (m *Model) openRunVersionSelect() tea.Cmd {
	m.selectRequest++
	m.selectKind = selectRunProfileVersion
	m.selectTitle = "Odoo version · " + m.taskProfileName
	m.selectItems = nil
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.modal = modalSelect
	return LoadVersionsCmd(m.ctx, m.service, m.selectRequest)
}

func (m *Model) openCreateProfileForm(version string) {
	m.createProfileVersion = version
	m.createProfileName = ""
	m.createFormErr = nil
	m.formField = 0
	m.modal = modalCreateProfile
}

func (m *Model) submitCreateProfile() tea.Cmd {
	name := strings.TrimSpace(m.createProfileName)
	if err := profile.ValidateName(name); err != nil {
		m.createFormErr = err
		return nil
	}
	for _, existing := range m.profiles {
		if existing.Name == name {
			m.createFormErr = fmt.Errorf("profile %q already exists", name)
			return nil
		}
	}
	m.modal = modalNone
	return m.queueTask(taskRequest{Kind: taskCreate, ProfileName: name, Version: m.createProfileVersion})
}

func (m *Model) openProfileConfig() tea.Cmd {
	profileName := m.selectedProfileName()
	if profileName == "" || m.rejectMutatingAction() {
		return nil
	}
	m.configProfileName = profileName
	m.selectRequest++
	m.selectKind = selectConfigDBFilterMode
	m.selectTitle = "Database filter mode · " + profileName
	m.selectItems = nil
	m.selectIndex = 0
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.modal = modalSelect
	return LoadProfileConfigCmd(m.ctx, m.service, profileName, m.selectRequest)
}

func (m *Model) applyLoadedProfileConfig(config profile.Config) {
	m.configDBFilterMode = config.EffectiveDBFilterMode()
	m.configDBFilterPattern = config.DBFilterPattern
	m.configAdminPasswd = config.AdminPasswd
	m.configFormErr = nil
	m.profileSettingsIndex = 0
	m.modal = modalProfileSettings
}

type profileSettingRow uint8

const (
	settingDBFilterMode profileSettingRow = iota
	settingDBFilterPattern
	settingAdminPasswd
)

// profileSettingRows lists the settings that apply to the current mode. The
// pattern row only exists in custom mode, so the operator never edits a field
// that would be rejected.
func (m *Model) profileSettingRows() []profileSettingRow {
	rows := []profileSettingRow{settingDBFilterMode}
	if m.configDBFilterMode == profile.DBFilterModeCustom {
		rows = append(rows, settingDBFilterPattern)
	}
	rows = append(rows, settingAdminPasswd)
	return rows
}

func (m *Model) moveProfileSetting(delta int) {
	rows := m.profileSettingRows()
	if len(rows) == 0 {
		return
	}
	m.profileSettingsIndex = (m.profileSettingsIndex + delta + len(rows)) % len(rows)
}

func (m *Model) editProfileSetting() tea.Cmd {
	rows := m.profileSettingRows()
	if m.profileSettingsIndex < 0 || m.profileSettingsIndex >= len(rows) {
		return nil
	}
	m.configFormErr = nil
	switch rows[m.profileSettingsIndex] {
	case settingDBFilterMode:
		m.selectRequest++
		m.selectKind = selectConfigDBFilterMode
		m.selectTitle = "Database filter mode · " + m.configProfileName
		m.selectItems = m.configModeSelectItems()
		m.selectIndex = m.currentConfigModeIndex()
		m.selectErr = nil
		m.selectPhase = phaseReady
		m.modal = modalSelect
		return nil
	case settingDBFilterPattern:
		m.configPatternDraft = m.configDBFilterPattern
		m.formField = 0
		m.modal = modalConfigPattern
		return nil
	case settingAdminPasswd:
		m.configPasswordDraft = m.configAdminPasswd
		m.formField = 0
		m.modal = modalConfigPassword
		return nil
	}
	return nil
}

func (m *Model) configModeSelectItems() []selectItem {
	modes := []struct{ label, value string }{
		{"profile (only this profile's databases)", profile.DBFilterModeProfile},
		{"disabled (all databases visible)", profile.DBFilterModeDisabled},
		{"custom (regular expression)", profile.DBFilterModeCustom},
	}
	items := make([]selectItem, 0, len(modes))
	for _, mode := range modes {
		item := selectItem{Label: mode.label, Value: mode.value}
		if mode.value == m.configDBFilterMode {
			item.Hint = "current"
		}
		items = append(items, item)
	}
	return items
}

func (m *Model) currentConfigModeIndex() int {
	for index, item := range m.configModeSelectItems() {
		if item.Value == m.configDBFilterMode {
			return index
		}
	}
	return 0
}

// applySelectedConfigMode persists only the filter mode, except when custom is
// chosen without a pattern: then the pattern editor opens first so the change
// stays valid.
func (m *Model) applySelectedConfigMode(mode string) tea.Cmd {
	if mode == profile.DBFilterModeCustom && strings.TrimSpace(m.configDBFilterPattern) == "" {
		m.configDBFilterMode = mode
		m.configPatternDraft = ""
		m.formField = 0
		m.modal = modalConfigPattern
		return nil
	}
	selected := mode
	m.configDBFilterMode = mode
	m.modal = modalProfileSettings
	return m.queueTask(taskRequest{
		Kind:         taskUpdateConfig,
		ProfileName:  m.configProfileName,
		ConfigUpdate: &profile.ConfigUpdate{DBFilterMode: &selected},
	})
}

func (m *Model) submitConfigPattern() tea.Cmd {
	pattern := strings.TrimSpace(m.configPatternDraft)
	if pattern == "" {
		m.configFormErr = errors.New("custom filter mode requires a pattern")
		return nil
	}
	mode := profile.DBFilterModeCustom
	m.configDBFilterPattern = pattern
	m.configDBFilterMode = mode
	m.modal = modalProfileSettings
	return m.queueTask(taskRequest{
		Kind:        taskUpdateConfig,
		ProfileName: m.configProfileName,
		ConfigUpdate: &profile.ConfigUpdate{
			DBFilterMode:    &mode,
			DBFilterPattern: &pattern,
		},
	})
}

func (m *Model) submitConfigPassword() tea.Cmd {
	password := m.configPasswordDraft
	m.configAdminPasswd = password
	m.modal = modalProfileSettings
	return m.queueTask(taskRequest{
		Kind:         taskUpdateConfig,
		ProfileName:  m.configProfileName,
		ConfigUpdate: &profile.ConfigUpdate{AdminPasswd: &password},
	})
}

func (m *Model) openAdminPasswordForm() {
	if m.rejectMutatingAction() {
		return
	}
	if !m.prepareDatabaseAction() {
		m.notice = "select a database first"
		return
	}
	if !m.profileIsRunning() {
		m.notice = "the profile must be running to change its admin password"
		return
	}
	m.adminPassword = ""
	m.adminPasswordConfirm = ""
	m.adminPasswordErr = nil
	m.formField = 0
	m.modal = modalAdminPassword
}

func (m *Model) submitAdminPasswordForm() tea.Cmd {
	password := m.adminPassword
	if strings.TrimSpace(password) == "" {
		m.adminPasswordErr = errors.New("password cannot be empty")
		return nil
	}
	if password != m.adminPasswordConfirm {
		m.adminPasswordErr = errors.New("passwords do not match")
		return nil
	}
	m.adminPasswordErr = nil
	m.modal = modalNone
	return m.queueTask(taskRequest{
		Kind:             taskSetAdminPassword,
		ProfileName:      m.taskProfileName,
		DatabaseName:     m.taskDatabaseName,
		DatabasePhysical: m.taskDatabasePhysical,
		Password:         password,
	})
}

func (m *Model) refreshProfiles() tea.Cmd {
	m.profileListOnlyRefresh = false
	m.loading = true
	m.err = nil
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) updateModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalTypedConfirm:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
			m.confirmAction = nil
			m.confirmInput = ""
			m.confirmErr = nil
		case "enter":
			return m, m.submitTypedConfirm()
		case "backspace", "ctrl+h":
			m.confirmInput = removeLastRune(m.confirmInput)
			m.confirmErr = nil
		case "ctrl+u":
			m.confirmInput = ""
			m.confirmErr = nil
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.confirmInput += value
				m.confirmErr = nil
			}
		}
	case modalConfirmWorktreeAddon:
		switch msg.String() {
		case "esc", "n", "N":
			m.modal = modalWorktreeAddon
		case "enter", "y", "Y":
			request := taskRequest{
				Kind:        taskWorktreeAddon,
				AddonName:   strings.TrimSpace(m.worktreeAddonName),
				AddonSource: strings.TrimSpace(m.worktreeAddonSource),
				AddonBranch: strings.TrimSpace(m.worktreeAddonBranch),
				Yes:         true,
			}
			m.modal = modalNone
			return m, m.queueTask(request)
		}
	case modalConfirmAddonRemove:
		switch msg.String() {
		case "esc", "n", "N":
			m.modal = modalRemoveAddon
		case "enter", "y", "Y":
			request := taskRequest{
				Kind:      taskRemoveAddon,
				AddonName: strings.TrimSpace(m.removeAddonName),
				Force:     m.removeAddonForce,
				Yes:       true,
			}
			m.modal = modalNone
			return m, m.queueTask(request)
		}
	case modalAddonMount:
		switch msg.String() {
		case "esc", "n", "N":
			m.closeAddonMountChooser()
		case "up", "k":
			m.moveIndex(&m.addonChoiceIndex, len(m.addonChoices), -1)
		case "down", "j":
			m.moveIndex(&m.addonChoiceIndex, len(m.addonChoices), 1)
		case " ":
			if m.addonChoicePhase == phaseReady && m.addonChoiceIndex >= 0 && m.addonChoiceIndex < len(m.addonChoices) {
				name := m.addonChoices[m.addonChoiceIndex].Name
				m.addonChoiceSelected[name] = !m.addonChoiceSelected[name]
				m.addonChoiceErr = nil
			}
		case "tab":
			m.addonMountRecreate = !m.addonMountRecreate
		case "enter":
			return m, m.submitAddonMount()
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
			if m.versionPurpose == versionForCreate {
				m.openCreateProfileForm(version)
				return m, nil
			}
			m.modal = modalNone
			return m, m.queueTask(taskRequest{Kind: taskRun, ProfileName: m.taskProfileName, Version: version, Wait: true})
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
	case modalFilePicker:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
		case "up", "k":
			m.moveIndex(&m.filePickerIndex, len(m.filePickerItems), -1)
		case "down", "j":
			m.moveIndex(&m.filePickerIndex, len(m.filePickerItems), 1)
		case "backspace", "left", "h":
			return m, m.filePickerParent()
		case "enter":
			return m, m.chooseFilePickerEntry()
		}
	case modalSelect:
		switch msg.String() {
		case "esc":
			m.closeSelect()
		case "up", "k":
			m.moveIndex(&m.selectIndex, len(m.selectItems), -1)
		case "down", "j":
			m.moveIndex(&m.selectIndex, len(m.selectItems), 1)
		case "enter":
			return m, m.applySelectChoice()
		}
	case modalInitDatabase, modalBackupDatabase, modalRestoreDatabase:
		if m.modal == modalRestoreDatabase {
			m.restoreErr = nil
		}
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
	case modalCloneAddon, modalWorktreeAddon, modalRemoveAddon:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
		case "enter":
			return m, m.submitAddonForm()
		case "tab":
			m.moveFormField(1)
		case "shift+tab":
			m.moveFormField(-1)
		case "backspace", "ctrl+h":
			m.removeFormText()
			m.addonFormErr = nil
		case "ctrl+u":
			m.clearFormText()
			m.addonFormErr = nil
		case " ":
			if !m.toggleFormField() {
				m.appendFormText(" ")
			}
			m.addonFormErr = nil
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.appendFormText(value)
				m.addonFormErr = nil
			}
		}
	case modalProfileSettings:
		switch msg.String() {
		case "esc", "q":
			m.modal = modalNone
		case "up", "k":
			m.moveProfileSetting(-1)
		case "down", "j":
			m.moveProfileSetting(1)
		case "enter":
			return m, m.editProfileSetting()
		}
	case modalConfigPattern, modalConfigPassword:
		switch msg.String() {
		case "esc":
			m.modal = modalProfileSettings
		case "enter":
			return m, m.submitProfileForm()
		case "tab":
			m.moveFormField(1)
		case "shift+tab":
			m.moveFormField(-1)
		case "backspace", "ctrl+h":
			m.removeFormText()
			m.configFormErr = nil
		case "ctrl+u":
			m.clearFormText()
			m.configFormErr = nil
		case " ":
			if !m.toggleFormField() {
				m.appendFormText(" ")
			}
			m.configFormErr = nil
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.appendFormText(value)
				m.configFormErr = nil
			}
		}
	case modalCreateProfile, modalAdminPassword:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
		case "enter":
			return m, m.submitProfileForm()
		case "tab":
			m.moveFormField(1)
		case "shift+tab":
			m.moveFormField(-1)
		case "backspace", "ctrl+h":
			m.removeFormText()
			m.clearProfileFormErr()
		case "ctrl+u":
			m.clearFormText()
			m.clearProfileFormErr()
		case " ":
			if !m.toggleFormField() {
				m.appendFormText(" ")
			}
			m.clearProfileFormErr()
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.appendFormText(value)
				m.clearProfileFormErr()
			}
		}
	}
	return m, nil
}

func (m *Model) submitAddonForm() tea.Cmd {
	switch m.modal {
	case modalCloneAddon:
		name := strings.TrimSpace(m.cloneAddonName)
		url := strings.TrimSpace(m.cloneAddonURL)
		if name == "" || url == "" {
			m.addonFormErr = errors.New("add-on name and Git URL are required")
			return nil
		}
		depth := 0
		if value := strings.TrimSpace(m.cloneAddonDepth); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				m.addonFormErr = errors.New("clone depth must be a non-negative number")
				return nil
			}
			depth = parsed
		}
		request := taskRequest{
			Kind:      taskAddAddon,
			AddonName: name,
			AddonURL:  url,
			AddonOptions: addons.AddOptions{
				Depth:  depth,
				Branch: strings.TrimSpace(m.cloneAddonBranch),
			},
		}
		m.modal = modalNone
		return m.queueTask(request)
	case modalWorktreeAddon:
		source := strings.TrimSpace(m.worktreeAddonSource)
		name := strings.TrimSpace(m.worktreeAddonName)
		branch := strings.TrimSpace(m.worktreeAddonBranch)
		if source == "" || name == "" || branch == "" {
			m.addonFormErr = errors.New("source add-on, worktree name, and branch are required")
			return nil
		}
		m.addonFormErr = nil
		m.modal = modalConfirmWorktreeAddon
	case modalRemoveAddon:
		if strings.TrimSpace(m.removeAddonName) == "" {
			m.addonFormErr = errors.New("add-on name is required")
			return nil
		}
		m.addonFormErr = nil
		m.modal = modalConfirmAddonRemove
	}
	return nil
}

func (m *Model) submitProfileForm() tea.Cmd {
	switch m.modal {
	case modalCreateProfile:
		return m.submitCreateProfile()
	case modalConfigPattern:
		return m.submitConfigPattern()
	case modalConfigPassword:
		return m.submitConfigPassword()
	case modalAdminPassword:
		return m.submitAdminPasswordForm()
	}
	return nil
}

func (m *Model) clearProfileFormErr() {
	switch m.modal {
	case modalCreateProfile:
		m.createFormErr = nil
	case modalConfigPattern, modalConfigPassword:
		m.configFormErr = nil
	case modalAdminPassword:
		m.adminPasswordErr = nil
	}
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
		m.restoreErr = nil
		destination := strings.TrimSpace(m.restoreDestination)
		if destination == "" {
			m.restoreErr = errors.New("destination database is required")
			return nil
		}
		if strings.TrimSpace(m.restoreSource) == "" {
			m.restoreErr = errors.New("restore source is required")
			return nil
		}
		if m.restoreCopy && m.restoreDestinationExists(destination) && !m.restoreForce {
			m.restoreErr = errors.New("destination already exists; pick another name or enable overwrite")
			return nil
		}
		m.taskDatabaseName = destination
		m.taskDatabasePhysical = ""
		if m.restoreForce || !m.restoreCopy {
			m.openTypedConfirm(
				"Move or overwrite can replace existing database data.",
				destination,
				func() tea.Cmd { return m.queueRestoreTask() },
			)
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
	case modalCloneAddon:
		return 4
	case modalWorktreeAddon:
		return 3
	case modalRemoveAddon:
		return 2
	case modalInitDatabase:
		return 2
	case modalBackupDatabase:
		return 4
	case modalRestoreDatabase:
		return 6
	case modalCreateProfile:
		return 1
	case modalConfigPattern, modalConfigPassword:
		return 1
	case modalAdminPassword:
		return 2
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
	case modalRemoveAddon:
		if m.formField == 1 {
			m.removeAddonForce = !m.removeAddonForce
			return true
		}
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
		case 2:
			m.restoreCopy = !m.restoreCopy
			return true
		case 3:
			m.restoreForce = !m.restoreForce
			return true
		case 4:
			m.restoreNeutralize = !m.restoreNeutralize
			return true
		}
	}
	return false
}

func (m *Model) formText() *string {
	switch m.modal {
	case modalCloneAddon:
		switch m.formField {
		case 0:
			return &m.cloneAddonName
		case 1:
			return &m.cloneAddonURL
		case 2:
			return &m.cloneAddonDepth
		case 3:
			return &m.cloneAddonBranch
		}
	case modalWorktreeAddon:
		switch m.formField {
		case 0:
			return &m.worktreeAddonSource
		case 1:
			return &m.worktreeAddonName
		case 2:
			return &m.worktreeAddonBranch
		}
	case modalRemoveAddon:
		if m.formField == 0 {
			return &m.removeAddonName
		}
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
			return &m.restoreDestination
		case 1:
			return &m.restoreSource
		case 5:
			return &m.restoreJobs
		}
	case modalCreateProfile:
		if m.formField == 0 {
			return &m.createProfileName
		}
	case modalConfigPattern:
		if m.formField == 0 {
			return &m.configPatternDraft
		}
	case modalConfigPassword:
		if m.formField == 0 {
			return &m.configPasswordDraft
		}
	case modalAdminPassword:
		switch m.formField {
		case 0:
			return &m.adminPassword
		case 1:
			return &m.adminPasswordConfirm
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
	if m.rejectMutatingAction() {
		return nil
	}
	m.notice = ""
	m.taskID++
	m.pendingTask = &request
	m.activeTaskKind = request.Kind
	m.taskProfileName = request.ProfileName
	m.taskDatabaseName = request.DatabaseName
	m.taskDatabasePhysical = request.DatabasePhysical
	m.taskName = taskLabel(request.Kind)
	if request.ProfileName != "" {
		m.taskName += " " + request.ProfileName
	}
	if request.DatabaseName != "" {
		m.taskName += "/" + request.DatabaseName
	}
	if request.AddonName != "" {
		if request.ProfileName == "" {
			m.taskName += " " + request.AddonName
		} else {
			m.taskName += "/" + request.AddonName
		}
	} else if len(request.AddonNames) > 0 {
		m.taskName += ": " + strings.Join(request.AddonNames, ", ")
	}
	if request.AddonSource != "" {
		m.taskName += " from " + request.AddonSource
	}
	m.taskStatus = "starting"
	m.taskErr = nil
	m.taskNotice = ""
	m.taskOutput = ""
	m.taskStarting = true
	m.taskRunning = false
	m.taskCancelRequested = false
	taskBaseContext := m.ctx
	if taskBaseContext == nil {
		taskBaseContext = context.Background()
	}
	m.taskContext, m.taskCancel = context.WithCancel(taskBaseContext)
	return tea.Batch(StartTaskCmd(m.taskID), m.activitySpinner.Tick)
}

func (m *Model) taskActive() bool {
	return m.taskStarting || m.taskRunning
}

func (m *Model) rejectMutatingAction() bool {
	if !m.taskActive() {
		return false
	}
	m.taskNotice = "task already running"
	return true
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

func (m *Model) finishTask(status string, taskErr error) {
	if m.taskCancel != nil {
		m.taskCancel()
	}
	m.taskCancel = nil
	m.taskContext = nil
	m.taskProgress = nil
	m.pendingTask = nil
	m.taskStarting = false
	m.taskRunning = false
	m.taskNotice = ""
	m.taskStatus = status
	m.taskCancelRequested = false
	m.taskErr = taskErr
}

func (m *Model) retainInteractiveOutput(name string, taskErr error) {
	m.taskName = name
	m.taskStatus = "completed"
	if taskErr != nil {
		m.taskStatus = "failed"
	}
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

func appendBoundedOutput(existing, next string) string {
	if next == "" {
		return existing
	}
	combined := existing + next
	if len(combined) <= maxLogBufferSize {
		return combined
	}
	start := len(combined) - maxLogBufferSize
	for start < len(combined) && (combined[start]&0xc0) == 0x80 {
		start++
	}
	return combined[start:]
}

func (m *Model) profileLogBuffer(profileName string) *profileLogBuffer {
	if m.profileLogs == nil {
		m.profileLogs = make(map[string]*profileLogBuffer)
	}
	buffer := m.profileLogs[profileName]
	if buffer == nil {
		buffer = &profileLogBuffer{}
		m.profileLogs[profileName] = buffer
	}
	return buffer
}

func (m *Model) profileLogOutput(profileName string) string {
	if profileName == "" {
		return ""
	}
	return m.profileLogBuffer(profileName).container
}

func (m *Model) profileTaskOutput(profileName string) string {
	if profileName == "" {
		return ""
	}
	return m.profileLogBuffer(profileName).task
}

func (m *Model) logEntries(profileName string) []logEntry {
	if profileName == "" {
		return nil
	}
	buffer := m.profileLogBuffer(profileName)
	if buffer.entries == nil || buffer.dirty {
		buffer.entries = buildLogEntries(buffer.container, buffer.task)
		buffer.dirty = false
	}
	return buffer.entries
}

func (m *Model) appendTaskLog(profileName, output string) {
	if profileName == "" || output == "" {
		return
	}
	buffer := m.profileLogBuffer(profileName)
	buffer.task = appendBoundedOutput(buffer.task, output)
	buffer.dirty = true
}

func (m *Model) appendContainerLog(profileName, output string) {
	if profileName == "" || output == "" {
		return
	}
	buffer := m.profileLogBuffer(profileName)
	buffer.container = appendBoundedOutput(buffer.container, output)
	buffer.containerSnapshot = appendBoundedOutput(buffer.containerSnapshot, output)
	buffer.containerLoaded = true
	buffer.dirty = true
}

func (m *Model) flushPendingContainerLogs(profileName string) {
	if m.logPendingOutput == "" {
		return
	}
	buffer := m.profileLogBuffer(profileName)
	output := outputSuffix(buffer.containerSnapshot, m.logPendingOutput)
	buffer.container = appendBoundedOutput(buffer.container, output)
	buffer.containerSnapshot = appendBoundedOutput(buffer.containerSnapshot, output)
	buffer.containerLoaded = true
	buffer.dirty = true
	m.logPendingOutput = ""
}

func (m *Model) mergeContainerLogs(profileName, output string) {
	buffer := m.profileLogBuffer(profileName)
	snapshot := appendBoundedOutput("", output)
	if !buffer.containerLoaded {
		buffer.container = appendBoundedOutput(snapshot, buffer.container)
		buffer.containerLoaded = true
	} else {
		buffer.container = appendBoundedOutput(buffer.container, outputSuffix(buffer.containerSnapshot, snapshot))
	}
	buffer.containerSnapshot = snapshot
	buffer.dirty = true
}

func outputSuffix(previous, next string) string {
	if previous == "" || next == "" {
		return next
	}
	prefix := make([]int, len(next))
	for index := 1; index < len(next); index++ {
		matched := prefix[index-1]
		for matched > 0 && next[index] != next[matched] {
			matched = prefix[matched-1]
		}
		if next[index] == next[matched] {
			matched++
		}
		prefix[index] = matched
	}

	matched := 0
	for index := 0; index < len(previous); index++ {
		for matched > 0 && previous[index] != next[matched] {
			matched = prefix[matched-1]
		}
		if previous[index] == next[matched] {
			matched++
		}
		if matched == len(next) && index < len(previous)-1 {
			matched = prefix[matched-1]
		}
	}
	return next[matched:]
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
	m.stopLogStream()
	m.logRequest++
	if previousLogProfile != profileName {
		m.logPhase = phaseIdle
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
	if m.focus == focusProfiles {
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
	if m.focus == focusProfiles {
		if invalidation.Kind == app.ProfileRuntimeUnavailable {
			m.setProfileLogsUnavailable(invalidation.ProfileName)
		} else {
			commands = append(commands, m.beginLogLoad())
		}
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
		m.profiles[index].PendingRecreation = detail.PendingRecreation
		return
	}
}

func (m *Model) setProfileLogsUnavailable(profileName string) {
	m.stopLogStream()
	m.logRequest++
	m.logPhase = phaseUnavailable
	m.logErr = nil
	m.logResourceName = profileName
	m.logViewportReady = false
}

func (m *Model) beginLogLoad() tea.Cmd {
	m.stopLogStream()
	m.logRequest++
	m.logPhase = phaseLoading
	m.logErr = nil
	m.logSnapshotPending = true
	m.logPendingOutput = ""
	profileName := m.selectedProfileName()
	m.logResourceName = profileName
	if profileName == "" {
		m.logPhase = phaseEmpty
		m.logSnapshotPending = false
		return nil
	}
	if !m.profileIsRunning() {
		m.logPhase = phaseUnavailable
		m.logSnapshotPending = false
		return nil
	}
	baseContext := m.ctx
	if baseContext == nil {
		baseContext = context.Background()
	}
	m.logContext, m.logCancel = context.WithCancel(baseContext)
	progress := make(chan profileLogStreamEvent, 32)
	m.logProgress = progress
	return tea.Batch(
		LoadProfileLogsCmd(m.logContext, m.service, profileName, m.logRequest),
		FollowProfileLogsCmd(m.logContext, m.service, profileName, m.logRequest, progress),
		WaitProfileLogStreamCmd(profileName, m.logRequest, progress),
	)
}

func (m *Model) stopLogStream() {
	if m.logCancel != nil {
		m.logCancel()
	}
	m.logContext = nil
	m.logCancel = nil
	m.logProgress = nil
	m.logSnapshotPending = false
	m.logPendingOutput = ""
}

func (m *Model) moveFocus(delta int) tea.Cmd {
	if m.selectedProfileName() == "" {
		return m.setFocus(focusProfiles)
	}
	return m.setFocus(focusArea((int(m.focus) + delta + int(focusAreaCount)) % int(focusAreaCount)))
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

func (m *Model) currentAddonChoiceRequest(requestID uint64, profileName string) bool {
	return requestID == m.addonChoiceRequest && m.modal == modalAddonMount &&
		m.addonMountAttach && profileName == m.addonChoiceProfile &&
		profileName == m.selectedProfileName()
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
	if m.logViewerOpen {
		view := m.logViewerView()
		if m.modal != modalNone {
			return m.modalOverlay(m.fitView(view))
		}
		return m.fitView(view)
	}
	if m.width < 72 || m.height < 14 {
		view := m.smallView()
		if m.modal != modalNone {
			return m.modalOverlay(m.fitView(view))
		}
		return m.fitView(view)
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
		return m.fitView(m.smallView())
	}

	mainHeight := m.height - 2 // separator and footer
	left := lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth).Height(mainHeight).MaxHeight(mainHeight).Render(m.leftView(leftWidth))
	right := lipgloss.NewStyle().Width(rightWidth).MaxWidth(rightWidth).Height(mainHeight).MaxHeight(mainHeight).Render(m.rightView(rightWidth))
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	footer := m.footerView()
	view := main + "\n" + mutedStyle.Render(strings.Repeat("─", m.width)) + "\n" + footer
	if m.modal != modalNone {
		return m.modalOverlay(m.fitView(view))
	}
	return m.fitView(view)
}

// fitView keeps the frame inside the terminal: no line wider than the window
// (which would wrap and desynchronize Bubble Tea's renderer) and no more lines
// than the window height. Long task output and log lines are the usual cause of
// the corruption seen when switching panels.
func (m *Model) fitView(view string) string {
	if m.width <= 0 || m.height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) modalOverlay(background string) string {
	base := strings.Split(lipgloss.NewStyle().Faint(true).Render(background), "\n")
	modal := lipgloss.NewStyle().
		Padding(0, 1).
		Render(m.modalView())
	modalLines := strings.Split(modal, "\n")
	modalWidth := lipgloss.Width(modal)
	left := (m.width - modalWidth) / 2
	top := (m.height - len(modalLines)) / 2
	if left < 0 {
		left = 0
	}
	for row := range base {
		if row < top || row >= top+len(modalLines) {
			base[row] = lipgloss.NewStyle().Width(m.width).Render(base[row])
			continue
		}
		modalLine := modalLines[row-top]
		base[row] = ansi.Cut(base[row], 0, left) + modalLine + ansi.Cut(base[row], left+modalWidth, m.width)
	}
	return m.fitView(strings.Join(base, "\n"))
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
	borderColor := borderStyle
	if selected {
		borderColor = inspectedBorderStyle
		if m.focus == focusProfiles {
			borderColor = activeBorderStyle
		}
	}

	name := profile.Name
	if selected {
		name = activeStyle.Render(name)
	}
	border := lipgloss.RoundedBorder()
	borderLine := lipgloss.NewStyle().Foreground(borderColor)
	horizontal := borderLine.Render(strings.Repeat(border.Top, cardWidth))
	vertical := borderLine.Render(border.Left)
	rightVertical := borderLine.Render(border.Right)
	headerRows := cardRows(cardHeader(name, profileState(profile.State), cardWidth), cardWidth)
	footerRows := cardRows(m.profileCardStatus(profile), cardWidth)
	lines := []string{borderLine.Render(border.TopLeft) + horizontal + borderLine.Render(border.TopRight)}
	for _, row := range headerRows {
		lines = append(lines, vertical+row+rightVertical)
	}
	lines = append(lines, borderLine.Render("├"+strings.Repeat(border.Top, cardWidth)+"┤"))
	for _, row := range footerRows {
		lines = append(lines, vertical+row+rightVertical)
	}
	lines = append(lines, borderLine.Render(border.BottomLeft)+horizontal+borderLine.Render(border.BottomRight))
	return strings.Join(lines, "\n")
}

func cardRows(value string, width int) []string {
	wrapped := lipgloss.NewStyle().Width(width).Render(value)
	lines := strings.Split(wrapped, "\n")
	for index, line := range lines {
		padding := width - lipgloss.Width(line)
		if padding > 0 {
			lines[index] += strings.Repeat(" ", padding)
		}
	}
	return lines
}

func cardHeader(name, state string, width int) string {
	gap := width - lipgloss.Width(name) - lipgloss.Width(state)
	if gap < 1 {
		gap = 1
	}
	return name + strings.Repeat(" ", gap) + state
}

func (m *Model) profileCardStatus(profile docker.ProfileSummary) string {
	if m.taskActive() && m.taskProfileName == profile.Name {
		return activeStyle.Render(taskActionLabel(m.activeTaskKind)) + " " + m.activitySpinner.View()
	}
	if profile.PendingRecreation.Pending {
		return warningStyle.Render("pending recreation ")
	}
	return mutedStyle.Render(profile.URL)
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
	if m.selectedProfileName() == "" && !m.loading {
		height := m.height - 2
		if height < 1 {
			height = 1
		}
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Render("create your first container  ·  press c or n")
	}

	var content string
	switch m.focus {
	case focusProfiles:
		content = m.logView(width)
	case focusDatabases:
		content = m.databaseTabView()
	case focusAddons:
		content = m.addonTabView()
	default:
		content = m.profileDetailView()
	}
	content = m.tabBar() + "\n\n" + content
	warnings := make([]string, 0, 4)
	if m.loading {
		warnings = append(warnings, mutedStyle.Render("Loading profiles..."))
	}
	if m.err != nil {
		warnings = append(warnings, errorStyle.Render(m.err.Error()))
	}
	if m.notice != "" {
		warnings = append(warnings, warningStyle.Render(m.notice))
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

func (m *Model) tabBar() string {
	tabs := []struct {
		name  string
		focus focusArea
	}{
		{name: "Databases", focus: focusDatabases},
		{name: "Add-ons", focus: focusAddons},
		{name: "Info", focus: focusInfo},
	}
	labels := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		label := "  " + tab.name
		if m.focus == tab.focus {
			label = activeStyle.Render("  " + tab.name)
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, "    ")
}

func (m *Model) logView(width int) string {
	return m.logContainer(width, m.logContent())
}

func (m *Model) logContent() string {
	if m.selectedProfile() != nil && !m.profileIsRunning() {
		return mutedStyle.Render("start the container to see logs")
	}

	entries := filterLogEntriesByDatabase(containerLogEntries(m.displayLogEntries(m.selectedProfileName())), m.logDatabase)
	output := renderLogEntries(entries, false)
	if m.logDatabase != "" {
		note := mutedStyle.Render("db filter: " + m.logDatabase + "  ·  open logs (L) and press d to change")
		if output != "" {
			output = note + "\n" + output
		} else {
			output = note
		}
	}
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
		status = mutedStyle.Render("select a profile to load logs")
	}
	if status == "" {
		return output
	}
	if output == "" {
		return status
	}
	return status + "\n\n" + output
}

// displayLogEntries returns formatted entries for one profile, including the
// pending container bytes that arrived before the initial snapshot completed.
func (m *Model) displayLogEntries(profileName string) []logEntry {
	if m.logSnapshotPending && profileName == m.logResourceName {
		container := appendBoundedOutput(m.profileLogOutput(profileName), m.logPendingOutput)
		return buildLogEntries(container, m.profileTaskOutput(profileName))
	}
	return m.logEntries(profileName)
}

func (m *Model) filteredLogEntries(profileName string) []logEntry {
	entries := m.displayLogEntries(profileName)
	if len(entries) == 0 {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(m.logViewerSearch))
	filtered := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if !m.logViewerSource.allows(entry.Source) {
			continue
		}
		if !m.logViewerLevel.allows(entry.Level) {
			continue
		}
		if m.logViewerDatabase != "" && entry.Database != m.logViewerDatabase {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(entry.Text), query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// openLogDatabaseSelect lets the operator pick "all" or one database for the
// log panel, instead of cycling blindly.
func (m *Model) openLogDatabaseSelect() tea.Cmd {
	m.selectRequest++
	m.selectKind = selectLogDatabase
	m.selectTitle = "Log database · " + m.currentLogProfile()
	m.selectItems = m.logDatabaseSelectItems()
	m.selectIndex = m.currentLogDatabaseIndex()
	m.selectErr = nil
	m.selectPhase = phaseReady
	m.modal = modalSelect
	return nil
}

func (m *Model) currentLogProfile() string {
	if m.logViewerOpen && m.logViewerProfile != "" {
		return m.logViewerProfile
	}
	return m.selectedProfileName()
}

func (m *Model) logDatabaseSelectItems() []selectItem {
	seen := make(map[string]bool)
	databases := make([]string, 0)
	for _, database := range m.databases {
		if database.Physical != "" && !seen[database.Physical] {
			seen[database.Physical] = true
			databases = append(databases, database.Physical)
		}
	}
	for _, entry := range m.displayLogEntries(m.currentLogProfile()) {
		if entry.Database != "" && !seen[entry.Database] {
			seen[entry.Database] = true
			databases = append(databases, entry.Database)
		}
	}
	sort.Strings(databases)

	items := make([]selectItem, 0, len(databases)+1)
	items = append(items, selectItem{Label: "all databases", Value: ""})
	for _, database := range databases {
		items = append(items, selectItem{Label: database, Value: database})
	}
	return items
}

func (m *Model) currentLogDatabaseIndex() int {
	for index, item := range m.logDatabaseSelectItems() {
		if item.Value == m.logDatabase {
			return index
		}
	}
	return 0
}

func (m *Model) openLogViewer() {
	profileName := m.selectedProfileName()
	if profileName == "" {
		return
	}
	m.logViewerOpen = true
	m.logViewerProfile = profileName
	m.logViewerSource = logSourceOnlyContainer
	m.logViewerLevel = logLevelFilterAll
	m.logViewerSearch = ""
	m.logViewerDatabase = m.logDatabase
	m.logViewerTyping = false
	m.logViewerFollow = true
	m.logViewerReady = false
}

func (m *Model) closeLogViewer() {
	m.logViewerOpen = false
	m.logViewerTyping = false
	m.logViewerSearch = ""
}

func (m *Model) updateLogViewerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.logViewerTyping {
		switch msg.String() {
		case "esc":
			m.logViewerTyping = false
			m.logViewerSearch = ""
		case "enter":
			m.logViewerTyping = false
		case "backspace", "ctrl+h":
			m.logViewerSearch = removeLastRune(m.logViewerSearch)
		case "ctrl+u":
			m.logViewerSearch = ""
		default:
			value := msg.String()
			runes := []rune(value)
			if len(runes) == 1 && unicode.IsPrint(runes[0]) {
				m.logViewerSearch += value
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "q", "L":
		m.closeLogViewer()
	case "/":
		m.logViewerTyping = true
	case "tab":
		m.logViewerSource = (m.logViewerSource + 1) % 3
	case "1":
		m.logViewerLevel = logLevelFilterAll
	case "2":
		m.logViewerLevel = logLevelFilterInfo
	case "3":
		m.logViewerLevel = logLevelFilterWarning
	case "4":
		m.logViewerLevel = logLevelFilterError
	case "f":
		m.logViewerFollow = !m.logViewerFollow
	case "d":
		return m, m.openLogDatabaseSelect()
	case "g":
		m.logViewerViewport.GotoTop()
		m.logViewerFollow = false
	case "G":
		m.logViewerViewport.GotoBottom()
		m.logViewerFollow = true
	case "up", "k":
		m.logViewerViewport.LineUp(1)
		m.logViewerFollow = false
	case "down", "j":
		m.logViewerViewport.LineDown(1)
	case "pgup", "ctrl+u":
		m.logViewerViewport.HalfViewUp()
		m.logViewerFollow = false
	case "pgdown", "ctrl+d":
		m.logViewerViewport.HalfViewDown()
	}
	return m, nil
}

func (m *Model) logViewerView() string {
	width := m.width
	if width < 40 {
		width = 40
	}
	height := m.height
	if height < 12 {
		height = 12
	}

	sourceLabel := []string{"all", "container", "tasks"}[m.logViewerSource]
	levelLabel := []string{"all", "info+", "warn+", "error"}[m.logViewerLevel]
	database := "all"
	if m.logViewerDatabase != "" {
		database = m.logViewerDatabase
	}
	search := "—"
	if m.logViewerTyping {
		search = m.logViewerSearch + "▏"
	} else if m.logViewerSearch != "" {
		search = m.logViewerSearch
	}
	header := titleStyle.Render("Logs · "+m.logViewerProfile) +
		"   " + mutedStyle.Render("source") + " " + activeStyle.Render(sourceLabel) +
		"   " + mutedStyle.Render("level") + " " + activeStyle.Render(levelLabel) +
		"   " + mutedStyle.Render("db") + " " + activeStyle.Render(database) +
		"   " + mutedStyle.Render("search") + " " + activeStyle.Render(search)

	entries := m.filteredLogEntries(m.logViewerProfile)
	content := renderLogEntries(entries, m.logViewerSource == logSourceAll)
	if content == "" {
		content = mutedStyle.Render("no log lines match the current filters")
	}

	bodyHeight := height - 6
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderStyle).
		Padding(0, 1)
	wasBottom := !m.logViewerReady || m.logViewerViewport.AtBottom()
	m.logViewerViewport.Width = width - 4
	m.logViewerViewport.Height = bodyHeight
	m.logViewerViewport.Style = style
	m.logViewerViewport.SetContent(content)
	if m.logViewerFollow || wasBottom {
		m.logViewerViewport.GotoBottom()
	}
	m.logViewerReady = true

	footer := mutedStyle.Render("esc close  tab source  d db  / search  1 all  2 info+  3 warn+  4 error  f follow  g/G top/bottom  ↑/↓/wheel scroll")
	return header + "\n" + m.logViewerViewport.View() + "\n" + footer
}

func (m *Model) logContainer(width int, content string) string {
	height := m.height - 6
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
	title := "Databases: " + profileName
	if m.databasePhase == phaseReady && len(m.databases) > 0 {
		title += fmt.Sprintf("  (%d)", len(m.databases))
	}
	return strings.Join([]string{
		titleStyle.Render(title),
		m.databaseRows(),
		m.databaseDetailView(),
	}, "\n\n")
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
	if m.taskNotice != "" {
		lines = append(lines, errorStyle.Render(m.taskNotice))
	}
	if m.taskErr != nil {
		lines = append(lines, errorStyle.Render(m.taskErr.Error()))
	}
	if m.taskOutput != "" {
		lines = append(lines, mutedStyle.Render(taskOutputTail(m.taskOutput, 6)))
	}
	return strings.Join(lines, "\n")
}

func taskOutputTail(output string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
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
	keys := "tab/←/→ focus  L logs  ctrl+r refresh  ? help  q/ctrl+c quit"
	if m.focus == focusProfiles && m.selectedProfileName() == "" {
		keys = "c create  ctrl+r refresh  ? help  q/ctrl+c quit"
	}
	if m.focus == focusProfiles && m.selectedProfileName() != "" {
		keys = "tab/←/→ focus  ↑/↓ profiles  " + profileActionHints() + "  pgup/pgdn/scroll logs  ctrl+r refresh  ? help  q/ctrl+c quit"
	}
	if m.focus == focusDatabases && m.selectedProfileName() != "" {
		keys = "tab/←/→ focus  ↑/↓ databases  i init"
		if m.selectedDatabase() != nil {
			keys += "  " + databaseActionHints()
		}
		keys += "  L logs  ctrl+r refresh  ? help  q/ctrl+c quit"
	}
	if m.focus == focusAddons {
		keys = "tab/←/→ focus  ↑/↓ add-ons  " + addonActionHints(m.selectedAddon() != nil, m.selectedProfileName() != "") + "  L logs  ctrl+r refresh  ? help  q/ctrl+c quit"
	}
	if m.showHelp {
		keys = "tab/←/→ focus  ↑/↓/j/k move  L logs  ctrl+r refresh  q/ctrl+c quit  ? hide help"
		if m.focus == focusProfiles && m.selectedProfileName() != "" {
			keys = "tab/←/→ focus  ↑/↓/j/k profiles  " + profileActionHints() + "  pgup/pgdn scroll logs  ctrl+r refresh  ? hide help"
		}
		if m.focus == focusDatabases && m.selectedProfileName() != "" {
			keys = "tab/←/→ focus  ↑/↓/j/k databases  i init new database"
			if m.selectedDatabase() != nil {
				keys += "  " + databaseActionHints()
			}
			keys += "  L logs  ctrl+r refresh  ? hide help"
		}
		if m.focus == focusAddons {
			keys = "tab/←/→ focus  ↑/↓/j/k add-ons  " + addonActionHints(m.selectedAddon() != nil, m.selectedProfileName() != "") + "  L logs  ctrl+r refresh  ? hide help"
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
	return "enter open  space run/stop  n new  e settings  L logs  D logs db  x start  r restart  R recreate  s stop  d remove"
}

func databaseActionHints() string {
	return "u update  U update-all  d drop  b backup  R restore  p psql  a admin user password"
}

func addonActionHints(hasSelection, hasProfile bool) string {
	hints := "c clone  w worktree  x remove"
	if hasProfile {
		hints = "a attach  " + hints
	}
	if hasSelection {
		hints += "  d detach  f fetch  p pull"
	}
	return hints
}

func (m *Model) modalView() string {
	var lines []string
	switch m.modal {
	case modalTypedConfirm:
		lines = []string{
			titleStyle.Render("Confirm: " + m.confirmExpected),
			m.confirmPrompt,
			"",
			"Type " + activeStyle.Render(m.confirmExpected) + " to confirm:",
			"  " + m.confirmInput + "▏",
		}
		if m.confirmErr != nil {
			lines = append(lines, errorStyle.Render(m.confirmErr.Error()))
		}
		lines = append(lines, "", mutedStyle.Render("enter confirm  esc cancel"))
	case modalRunVersion:
		lines = []string{
			titleStyle.Render("Odoo version required"),
			"Profile: " + m.taskProfileName,
			"No stored Odoo version exists for this profile.",
			"version: " + activeStyle.Render(m.versionInput+"▏"),
			"",
			mutedStyle.Render("type a version  enter run  esc cancel"),
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
		destination := m.restoreDestination
		if destination == "" {
			destination = "(required)"
		}
		lines = []string{
			titleStyle.Render("Restore database"),
			"Profile: " + m.taskProfileName,
			m.formLine("destination", destination, 0, true),
			m.formLine("source", source, 1, true),
			m.formLine("mode", restoreMode(m.restoreCopy), 2, false),
			m.formLine("overwrite", yesNo(m.restoreForce), 3, false),
			m.formLine("neutralize", yesNo(m.restoreNeutralize), 4, false),
			m.formLine("jobs", m.restoreJobs, 5, true),
		}
		if m.restoreErr != nil {
			lines = append(lines, errorStyle.Render(m.restoreErr.Error()))
		}
		lines = append(lines,
			"",
			mutedStyle.Render("copy creates a new database (destination must not exist); move replaces it"),
			mutedStyle.Render("tab field  space toggle  enter run  esc cancel"),
		)
	case modalCloneAddon:
		lines = []string{
			titleStyle.Render("Clone and register add-on"),
			m.formLine("name", m.cloneAddonName, 0, true),
			m.formLine("Git URL", m.cloneAddonURL, 1, true),
			m.formLine("depth (0 = full)", m.cloneAddonDepth, 2, true),
			m.formLine("branch (optional)", m.cloneAddonBranch, 3, true),
		}
		if m.addonFormErr != nil {
			lines = append(lines, errorStyle.Render(m.addonFormErr.Error()))
		}
		lines = append(lines, "", mutedStyle.Render("tab field  type  enter clone  esc cancel"))
	case modalWorktreeAddon:
		lines = []string{
			titleStyle.Render("Create add-on worktree"),
			m.formLine("source add-on", m.worktreeAddonSource, 0, true),
			m.formLine("worktree name", m.worktreeAddonName, 1, true),
			m.formLine("branch", m.worktreeAddonBranch, 2, true),
		}
		if m.addonFormErr != nil {
			lines = append(lines, errorStyle.Render(m.addonFormErr.Error()))
		}
		lines = append(lines, "", mutedStyle.Render("tab field  type  enter continue  esc cancel"))
	case modalConfirmWorktreeAddon:
		lines = []string{
			titleStyle.Render("Confirm worktree creation"),
			"Source: " + strings.TrimSpace(m.worktreeAddonSource),
			"Name: " + strings.TrimSpace(m.worktreeAddonName),
			"Branch: " + strings.TrimSpace(m.worktreeAddonBranch),
			warningStyle.Render("If the branch is missing locally and remotely, create it."),
			"Existing parent/worktree rules still apply.",
			"",
			mutedStyle.Render("enter/y confirm  n/esc back"),
		}
	case modalRemoveAddon:
		lines = []string{
			titleStyle.Render("Remove add-on checkout"),
			m.formLine("name", m.removeAddonName, 0, true),
			m.formLine("force dirty checkout", yesNo(m.removeAddonForce), 1, false),
			warningStyle.Render("Attached profiles and registered child worktrees block removal."),
		}
		if m.addonFormErr != nil {
			lines = append(lines, errorStyle.Render(m.addonFormErr.Error()))
		}
		lines = append(lines, "", mutedStyle.Render("tab field  space toggle force  enter review  esc cancel"))
	case modalConfirmAddonRemove:
		lines = []string{
			titleStyle.Render("Remove add-on " + strings.TrimSpace(m.removeAddonName) + "?"),
			warningStyle.Render("This removes the checkout/worktree; attached profiles and child worktrees block removal."),
		}
		if m.removeAddonForce {
			lines = append(lines, warningStyle.Render("Force may discard uncommitted changes."))
		}
		lines = append(lines, "", mutedStyle.Render("enter/y confirm  n/esc back"))
	case modalSelect:
		lines = m.selectModalLines()
	case modalFilePicker:
		lines = m.filePickerModalLines()
	case modalCreateProfile:
		lines = []string{
			titleStyle.Render("New profile"),
			"Odoo version: " + activeStyle.Render(m.createProfileVersion),
			m.formLine("profile name", m.createProfileName, 0, true),
		}
		if m.createFormErr != nil {
			lines = append(lines, errorStyle.Render(m.createFormErr.Error()))
		}
		lines = append(lines,
			"",
			mutedStyle.Render("adds the profile to state with this version; start it later with space"),
			mutedStyle.Render("type a lowercase name  enter create  esc cancel"),
		)
	case modalProfileSettings:
		lines = m.profileSettingsModalLines()
	case modalConfigPattern:
		lines = []string{
			titleStyle.Render("Database filter pattern · " + m.configProfileName),
			m.formLine("regular expression", m.configPatternDraft, 0, true),
		}
		if m.configFormErr != nil {
			lines = append(lines, errorStyle.Render(m.configFormErr.Error()))
		}
		lines = append(lines,
			"",
			mutedStyle.Render("matched against physical database names, e.g. ^smoke__(dev|staging)$"),
			mutedStyle.Render("tab field  type  enter apply  esc back"),
		)
	case modalConfigPassword:
		lines = []string{
			titleStyle.Render("Odoo master password · " + m.configProfileName),
			m.formLine("master password", maskSecret(m.configPasswordDraft), 0, true),
		}
		if m.configFormErr != nil {
			lines = append(lines, errorStyle.Render(m.configFormErr.Error()))
		}
		lines = append(lines,
			"",
			mutedStyle.Render("this is admin_passwd (/web/database), not the admin user login password"),
			mutedStyle.Render("tab field  type  enter apply  esc back"),
		)
	case modalAdminPassword:
		lines = []string{
			titleStyle.Render("Change Odoo admin user password"),
			"Profile: " + m.taskProfileName,
			"Database: " + databaseLabel(odoo.Database{Logical: m.taskDatabaseName, Physical: m.taskDatabasePhysical}),
			"User: " + activeStyle.Render("admin") + mutedStyle.Render("  (base.user_admin login)"),
			m.formLine("new password", maskSecret(m.adminPassword), 0, true),
			m.formLine("confirm", maskSecret(m.adminPasswordConfirm), 1, true),
		}
		if m.adminPasswordErr != nil {
			lines = append(lines, errorStyle.Render(m.adminPasswordErr.Error()))
		}
		lines = append(lines,
			"",
			mutedStyle.Render("NOT admin_passwd; changes the login password of the admin user via Odoo"),
			mutedStyle.Render("tab field  type  enter apply  esc cancel"),
		)
	case modalAddonMount:
		lines = m.addonMountModalLines()
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
	lines = modalTitleSeparator(lines, modalWidth)
	if maxLines := m.height - 5; maxLines >= 3 && len(lines) > maxLines {
		lines = append(lines[:maxLines-1], mutedStyle.Render("…"))
	}
	return lipgloss.NewStyle().Width(modalWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(activeBorderStyle).Render(strings.Join(lines, "\n"))
}

// modalListBudget is how many list rows fit in a modal at the current height.
func (m *Model) modalListBudget() int {
	budget := m.height - 9
	if budget < 3 {
		budget = 3
	}
	return budget
}

// windowStart returns the first visible index so that index stays in view
// inside a list of count entries showing budget rows at once.
func windowStart(count, index, budget int) int {
	if count <= budget {
		return 0
	}
	start := index - budget/2
	if start < 0 {
		start = 0
	}
	if start+budget > count {
		start = count - budget
	}
	return start
}

// modalTitleSeparator draws a rule under the modal title so the title reads as
// a header instead of blending into the body.
func modalTitleSeparator(lines []string, width int) []string {
	if len(lines) < 2 || width < 1 {
		return lines
	}
	separatorWidth := width - 4 // modal horizontal padding
	if separatorWidth < 1 {
		separatorWidth = 1
	}
	separator := lipgloss.NewStyle().Foreground(activeBorderStyle).Render(strings.Repeat("─", separatorWidth))
	return append([]string{lines[0], separator}, lines[1:]...)
}

func (m *Model) addonMountModalLines() []string {
	action := "Attach"
	if !m.addonMountAttach {
		action = "Detach"
	}
	lines := []string{
		titleStyle.Render(action + " add-ons for " + m.addonChoiceProfile),
	}
	switch m.addonChoicePhase {
	case phaseLoading:
		lines = append(lines, mutedStyle.Render("Loading add-ons..."))
	case phaseEmpty:
		if m.addonMountAttach {
			lines = append(lines, mutedStyle.Render("No unattached add-ons available"))
		} else {
			lines = append(lines, mutedStyle.Render("No add-ons attached"))
		}
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.addonChoiceErr)))
	case phaseReady:
		budget := m.modalListBudget() - 2 // recreate line and hint below
		start := windowStart(len(m.addonChoices), m.addonChoiceIndex, budget)
		end := start + budget
		if end > len(m.addonChoices) {
			end = len(m.addonChoices)
		}
		if start > 0 {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
		}
		for index := start; index < end; index++ {
			addon := m.addonChoices[index]
			checked := "[ ] "
			if m.addonChoiceSelected[addon.Name] {
				checked = "[x] "
			}
			row := checked + addon.Name
			if !addon.PathAvailable {
				row += " (path unavailable)"
			}
			if index == m.addonChoiceIndex {
				lines = append(lines, activeStyle.Render("> "+row))
			} else {
				lines = append(lines, "  "+row)
			}
		}
		if end < len(m.addonChoices) {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.addonChoices)-end)))
		}
	default:
		lines = append(lines, mutedStyle.Render("Add-on list is not available"))
	}
	recreate := "no (leave pending)"
	if m.addonMountRecreate {
		recreate = "yes (apply now)"
	}
	lines = append(lines, "", "Recreate profile now: "+recreate)
	if m.addonChoiceErr != nil && m.addonChoicePhase != phaseError {
		lines = append(lines, errorStyle.Render(m.addonChoiceErr.Error()))
	}
	lines = append(lines, "", mutedStyle.Render("↑/↓ move  space select  tab toggle recreate  enter apply  esc cancel"))
	return lines
}

func (m *Model) profileSettingsModalLines() []string {
	lines := []string{titleStyle.Render("Profile settings · " + m.configProfileName)}
	rows := m.profileSettingRows()
	for index, row := range rows {
		label, value := m.profileSettingValue(row)
		line := label + ": " + value
		if index == m.profileSettingsIndex {
			lines = append(lines, activeStyle.Render("> "+line))
		} else {
			lines = append(lines, "  "+line)
		}
	}
	if m.configFormErr != nil {
		lines = append(lines, errorStyle.Render(m.configFormErr.Error()))
	}
	lines = append(lines,
		"",
		mutedStyle.Render("saved to .lidoo.json; recreate (R) applies it to a container"),
		mutedStyle.Render("↑/↓ move  enter edit  esc close"),
	)
	return lines
}

func (m *Model) profileSettingValue(row profileSettingRow) (string, string) {
	switch row {
	case settingDBFilterMode:
		return "database filter mode", m.configDBFilterMode
	case settingDBFilterPattern:
		if strings.TrimSpace(m.configDBFilterPattern) == "" {
			return "filter pattern", warningStyle.Render("(required)")
		}
		return "filter pattern", m.configDBFilterPattern
	case settingAdminPasswd:
		if m.configAdminPasswd == "" {
			return "Odoo master password", mutedStyle.Render("(not set)")
		}
		return "Odoo master password", runningStyle.Render("set")
	}
	return "", ""
}

func (m *Model) filePickerModalLines() []string {
	lines := []string{
		titleStyle.Render("Restore source · " + m.taskDatabaseName),
		mutedStyle.Render(m.filePickerPath),
		"",
	}
	switch m.filePickerPhase {
	case phaseLoading:
		lines = append(lines, mutedStyle.Render("Loading…"))
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.filePickerErr)))
	case phaseEmpty:
		lines = append(lines, mutedStyle.Render("No folders or .zip/.dump files here"))
	case phaseReady:
		budget := m.modalListBudget()
		start := windowStart(len(m.filePickerItems), m.filePickerIndex, budget)
		end := start + budget
		if end > len(m.filePickerItems) {
			end = len(m.filePickerItems)
		}
		if start > 0 {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
		}
		for index := start; index < end; index++ {
			entry := m.filePickerItems[index]
			label := entry.Name
			if entry.Dir {
				label += "/"
			}
			if index == m.filePickerIndex {
				lines = append(lines, activeStyle.Render("> "+label))
			} else {
				lines = append(lines, "  "+label)
			}
		}
		if end < len(m.filePickerItems) {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.filePickerItems)-end)))
		}
	default:
		lines = append(lines, mutedStyle.Render("not loaded"))
	}
	lines = append(lines, "", mutedStyle.Render("↑/↓ move  enter open/select  h/backspace up  esc cancel"))
	return lines
}

func (m *Model) selectModalLines() []string {
	lines := []string{titleStyle.Render(m.selectTitle)}
	switch m.selectPhase {
	case phaseLoading:
		lines = append(lines, mutedStyle.Render("Loading options..."))
	case phaseEmpty:
		lines = append(lines, mutedStyle.Render("Nothing to choose"))
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.selectErr)))
	case phaseReady:
		budget := m.modalListBudget()
		start := windowStart(len(m.selectItems), m.selectIndex, budget)
		end := start + budget
		if end > len(m.selectItems) {
			end = len(m.selectItems)
		}
		if start > 0 {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
		}
		for index := start; index < end; index++ {
			item := m.selectItems[index]
			label := item.Label
			if item.Hint != "" {
				label += "  " + mutedStyle.Render(item.Hint)
			}
			if index == m.selectIndex {
				lines = append(lines, activeStyle.Render("> "+label))
			} else {
				lines = append(lines, "  "+label)
			}
		}
		if end < len(m.selectItems) {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.selectItems)-end)))
		}
	default:
		lines = append(lines, mutedStyle.Render("Options are not available"))
	}
	if m.selectErr != nil && m.selectPhase == phaseReady {
		lines = append(lines, errorStyle.Render(m.selectErr.Error()))
	}
	lines = append(lines, "", mutedStyle.Render("↑/↓ move  enter select  esc cancel"))
	return lines
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

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	return strings.Repeat("•", len([]rune(value)))
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
	if m.focus == focusAddons && m.selectedProfileName() != "" {
		lines = append(lines, "", m.addonTabView())
	}
	hints := profileActionHints()
	if m.focus == focusDatabases {
		hints = "i init new database"
		if m.selectedDatabase() != nil {
			hints = databaseActionHints()
		}
	}
	if m.focus == focusAddons {
		hints = addonActionHints(m.selectedAddon() != nil, m.selectedProfileName() != "")
	}
	lines = append(lines, "", mutedStyle.Render(hints+"  ctrl+r refresh  ? help  q/ctrl+c quit"))
	if task := m.taskView(); task != "" {
		lines = append(lines, "", task)
	}
	if m.showHelp {
		lines = append(lines, mutedStyle.Render("tab focus  arrows or j/k move  c cancel task"))
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

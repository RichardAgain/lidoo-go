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
	"time"

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

type modalMode uint8

const (
	modalNone modalMode = iota
	modalConfirm
	modalRunVersion
	modalInitDatabase
	modalBackupDatabase
	modalRestoreDatabase
	modalCloneAddon
	modalWorktreeAddon
	modalSelect
	modalCreateProfile
	modalConfigPattern
	modalConfigPassword
	modalConfigLANPort
	modalAdminPassword
	modalFilePicker
	modalKeys
)

type selectKind uint8

const (
	selectNone selectKind = iota
	selectRunProfileVersion
	selectCreateProfileVersion
	selectAddonWorktreeSource
	selectAddonWorktreeBranch
	selectLogDatabase
)

// selectCustomValue is the sentinel value for "type a value instead of
// choosing one" entries in a select modal.
const selectCustomValue = "\x00custom"

// pasteHint is the explicit paste shortcut shown in every text form, so nobody
// has to guess whether pasting works.
const pasteHint = "ctrl+v paste"

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
	pane                   focusPane
	tab                    tabID
	loading                bool
	err                    error
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
	databaseInfos          map[string]odoo.DatabaseInfo
	addonPhase             resourcePhase
	addonErr               error
	addonFormErr           error
	cloneAddonName         string
	cloneAddonURL          string
	cloneAddonDepth        string
	cloneAddonBranch       string
	worktreeAddonSource    string
	worktreeAddonName      string
	worktreeAddonBranch    string
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
	taskAddonNames         []string
	taskAddonRecreate      bool
	taskStatus             string
	taskErr                error
	taskNotice             string
	taskOutput             string
	taskExpanded           bool
	taskStartedAt          time.Time
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
	selectFilter           listFilter
	selectPhase            resourcePhase
	selectErr              error
	selectRequest          uint64
	selectSource           string
	versionPurpose         versionPurpose
	filePickerPath         string
	filePickerItems        []files.DirEntry
	filePickerIndex        int
	filePickerFilter       listFilter
	filePickerDirectory    bool
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
	configPhase            resourcePhase
	configErr              error
	configRequest          uint64
	databaseTableTop       int
	databaseTableStart     int
	addonTableStart        int
	configPatternDraft     string
	configPasswordDraft    string
	configLANPortDraft     string
	adminPassword          string
	adminPasswordConfirm   string
	adminPasswordErr       error
	confirmAction          func() tea.Cmd
	confirmPrompt          string
	confirmTitle           string
	confirmCancelModal     modalMode
	confirmForceToggle     bool
}

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
	case ClipboardContentMsg:
		m.applyClipboardPaste(msg)
		return m, nil
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
		m.setProfiles(msg.Profiles)
		m.loading = false
		m.err = nil
		if listOnly && m.selectedProfileName() == m.profileResourceName {
			return m, nil
		}
		if m.selectedProfileName() != "" && m.selectedProfileName() == m.profileResourceName {
			return m, m.refreshProfileResources()
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
		if m.tab == tabLogs && !m.profileIsRunning() {
			m.setProfileLogsUnavailable(msg.ProfileName)
		}
		return m, m.afterProfileDetailLoaded()
	case ProfileDetailFailedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.pendingDatabaseRefresh = false
		m.profileDetailErr = msg.Err
		if m.profilePhase != phaseReady {
			m.profilePhase = phaseError
		}
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
			m.databaseInfos = nil
			return m, nil
		}
		m.databasePhase = phaseReady
		return m, m.beginDatabaseInfosLoad()
	case DatabasesFailedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		m.databaseErr = msg.Err
		if m.databasePhase != phaseReady && m.databasePhase != phaseEmpty {
			m.databaseInfoPhase = phaseUnavailable
			if m.profileIsRunning() {
				m.databasePhase = phaseError
			} else {
				m.databasePhase = phaseUnavailable
			}
		}
		return m, nil
	case DatabaseInfosLoadedMsg:
		if msg.RequestID != m.databaseRequest || msg.ProfileName != m.selectedProfileName() {
			return m, nil
		}
		m.databaseInfos = make(map[string]odoo.DatabaseInfo, len(msg.Results))
		for _, result := range msg.Results {
			if result.Err != nil {
				continue
			}
			m.databaseInfos[result.DatabasePhysical] = result.Info
		}
		m.databaseInfoPhase = phaseReady
		return m, nil
	case AddonsLoadedMsg:
		if !m.currentProfileRequest(msg.RequestID, msg.ProfileName) {
			return m, nil
		}
		selectedAddonName := ""
		if selected := m.selectedAddon(); selected != nil {
			selectedAddonName = selected.Name
		}
		m.addons = append([]addons.AddonStatus(nil), msg.Addons...)
		m.addonIndex = min(m.addonIndex, max(0, len(m.addons)-1))
		for index, addon := range m.addons {
			if addon.Name == selectedAddonName {
				m.addonIndex = index
				break
			}
		}
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
		m.addonErr = msg.Err
		if m.addonPhase != phaseReady && m.addonPhase != phaseEmpty {
			m.addonPhase = phaseError
		}
		return m, nil
	case VersionsLoadedMsg:
		if msg.RequestID == m.selectRequest && m.modal == modalCreateProfile {
			m.setVersionSelectItems(msg.Options)
			if m.createProfileVersion == "" && len(msg.Options) > 0 {
				m.createProfileVersion = msg.Options[0].Version
			}
			return m, nil
		}
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
		if msg.RequestID == m.selectRequest && m.modal == modalCreateProfile {
			m.selectErr = msg.Err
			m.selectPhase = phaseError
			return m, nil
		}
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
		if msg.RequestID != m.configRequest || msg.ProfileName != m.selectedProfileName() {
			return m, nil
		}
		m.configProfileName = msg.ProfileName
		m.applyLoadedProfileConfig(msg.Config)
		return m, nil
	case ProfileConfigFailedMsg:
		if msg.RequestID != m.configRequest || msg.ProfileName != m.selectedProfileName() {
			return m, nil
		}
		m.configErr = msg.Err
		if m.configPhase != phaseReady {
			m.configPhase = phaseError
		}
		return m, nil
	case AllAddonsLoadedMsg:
		if !m.currentSelectRequest(msg.RequestID) {
			return m, nil
		}
		switch m.selectKind {
		case selectAddonWorktreeSource:
			m.setAddonSelectItems(msg.Addons, true)
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
		for _, entry := range msg.Entries {
			if !m.filePickerDirectory || entry.Dir {
				items = append(items, entry)
			}
		}
		m.filePickerItems = items
		selectFirstIndex(&m.filePickerIndex, m.filePickerIndices())
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
		kind := m.activeTaskKind
		addonNames := append([]string(nil), m.taskAddonNames...)
		recreate := m.taskAddonRecreate
		m.finishTask("completed", nil)
		if !recreate && (kind == taskAttachAddons || kind == taskDetachAddons) {
			return m, m.refreshAfterAddonMountTask(msg.ProfileName, kind == taskAttachAddons, addonNames)
		}
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
	if msg.String() == "ctrl+v" && m.pasteTarget() != nil {
		return m, ReadClipboardCmd()
	}
	if m.modal != modalNone {
		return m.updateModalKey(msg)
	}
	if m.logViewerOpen {
		return m.updateLogViewerKey(msg)
	}
	m.notice = ""
	if msg.String() == "t" && !m.taskActive() && !m.interactivePreparing && m.taskStatus != "" && m.taskName != "" {
		m.taskExpanded = !m.taskExpanded
		return m, nil
	}
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
	if m.tab == tabLogs {
		switch msg.String() {
		case "pgup", "pgdown", "f", "b", "ctrl+u", "ctrl+d":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			return m, cmd
		}
	}

	if cmd, handled := m.navigationKey(msg.String()); handled {
		return m, cmd
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "enter":
		if m.onTab(tabConfig) {
			return m, m.editProfileSetting()
		} else if m.onTab(tabAddons) {
			addon := m.selectedAddon()
			profileName := m.selectedProfileName()
			if addon == nil || profileName == "" || m.rejectMutatingAction() {
				return m, nil
			}
			kind := taskAttachAddons
			if addonAttached(*addon, profileName) {
				kind = taskDetachAddons
			}
			return m, m.queueTask(taskRequest{
				Kind: kind, ProfileName: profileName, AddonNames: []string{addon.Name},
			})
		}
	case "x":
		if m.onProfiles() && m.selectedProfileName() != "" {
			if m.profileIsRunning() {
				m.notice = "profile " + m.selectedProfileName() + " is already running"
				return m, nil
			}
			return m, m.queueTask(taskRequest{Kind: taskRun, ProfileName: m.selectedProfileName(), Wait: true})
		} else if m.onTab(tabAddons) {
			return m, m.startRemoveAddon()
		}
	case "r":
		if m.onProfiles() && m.selectedProfileName() != "" {
			return m, m.queueTask(taskRequest{Kind: taskRestart, ProfileName: m.selectedProfileName()})
		}
		return m, m.refreshProfiles()
	case "R":
		if (m.onProfiles() || m.onTab(tabConfig)) && m.selectedProfileName() != "" {
			return m, m.queueTask(taskRequest{Kind: taskRecreate, ProfileName: m.selectedProfileName()})
		}
		if m.onTab(tabDatabases) {
			return m, m.startRestorePicker()
		}
	case "s":
		if m.onProfiles() && m.selectedProfileName() != "" {
			if !m.profileIsRunning() {
				m.notice = "profile " + m.selectedProfileName() + " is not running"
				return m, nil
			}
			return m, m.queueTask(taskRequest{Kind: taskStop, ProfileName: m.selectedProfileName()})
		}
	case " ":
		if m.onProfiles() && m.selectedProfileName() != "" {
			return m, m.toggleSelectedProfile()
		}
	case "/":
		if m.tab == tabLogs && m.selectedProfileName() != "" {
			m.openLogViewer()
			m.logViewerTyping = true
		}
	case "L":
		if m.selectedProfileName() != "" {
			if m.onTab(tabDatabases) {
				if database := m.selectedDatabase(); database != nil {
					m.logDatabase = database.Physical
				}
			}
			m.openLogViewer()
		}
	case "D":
		if (m.onProfiles() || m.onTab(tabLogs)) && m.selectedProfileName() != "" {
			return m, m.openLogDatabaseSelect()
		}
	case "d":
		if m.onProfiles() && m.selectedProfileName() != "" {
			if m.rejectMutatingAction() {
				return m, nil
			}
			name := m.selectedProfileName()
			m.taskProfileName = name
			m.openConfirm(
				"Remove profile "+name+"?",
				"This removes the container, workspace entry, and filestore volume; PostgreSQL data and add-on checkouts are kept.",
				modalNone,
				func() tea.Cmd {
					return m.queueTask(taskRequest{Kind: taskRemove, ProfileName: name})
				},
			)
		} else if m.onTab(tabDatabases) {
			m.openDropConfirmation()
		}
	case "a", "A":
		if m.onTab(tabDatabases) {
			m.openAdminPasswordForm()
		}
	case "c":
		if m.onProfiles() {
			return m, m.startCreateProfile()
		}
		if m.onTab(tabAddons) {
			m.openCloneAddonForm()
		}
	case "w":
		if m.onTab(tabAddons) {
			return m, m.startWorktreeAddon()
		}
	case "n":
		if m.onProfiles() {
			return m, m.startCreateProfile()
		}
	case "e":
		if m.onTab(tabConfig) {
			return m, m.editProfileSetting()
		}
		return m, m.openTab(tabConfig)
	case "f":
		if m.onTab(tabAddons) {
			return m, m.queueSelectedAddonTask(taskFetchAddon)
		}
	case "i":
		if m.onTab(tabDatabases) {
			m.openInitForm()
		}
	case "u":
		if m.onTab(tabDatabases) {
			return m, m.queueSelectedDatabaseTask(taskUpdate, false)
		}
	case "U":
		if m.onTab(tabDatabases) {
			return m, m.queueSelectedDatabaseTask(taskUpdate, true)
		}
	case "b":
		if m.onTab(tabDatabases) {
			m.openBackupForm()
		}
	case "p":
		if m.onTab(tabDatabases) {
			if m.rejectMutatingAction() {
				return m, nil
			}
			database := m.selectedDatabase()
			if database != nil {
				profileName := m.selectedProfileName()
				m.interactivePreparing = true
				m.taskName = "psql " + profileName + "/" + database.Logical
				m.taskStatus = "preparing"
				m.taskStartedAt = time.Now()
				m.taskErr = nil
				m.err = nil
				return m, PrepareDatabaseShellCmd(m.ctx, m.service, profileName, database.Logical)
			}
		} else if m.onTab(tabAddons) {
			return m, m.queueSelectedAddonTask(taskPullAddon)
		}
	case "ctrl+r":
		return m, m.refreshProfiles()
	case "?":
		m.modal = modalKeys
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
	if m.tab == tabLogs {
		var cmd tea.Cmd
		m.logViewport, cmd = m.logViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleLeftClick(x, y int) tea.Cmd {
	geometry := m.frame()
	inList := x < geometry.leftWidth
	if geometry.compact {
		inList = y < geometry.detailTop
	}
	if inList {
		row := y - geometry.listTop
		index := m.profileWindowStart() + row
		if row < 0 || row >= geometry.listRows || index >= len(m.profiles) {
			return nil
		}
		m.focusProfileList()
		if index == m.profileIndex {
			return nil
		}
		m.profileIndex = index
		m.profileListOnlyRefresh = false
		return m.beginProfileResources()
	}

	if m.selectedProfileName() == "" {
		return nil
	}
	tabY := geometry.detailTop + m.profileHeaderLines()
	reached := x - geometry.detailLeft
	if y == tabY {
		offset := 1
		for index, tab := range detailTabs {
			width := lipgloss.Width(tab.key + " " + tab.name)
			if reached >= offset && reached < offset+width {
				return m.openTab(tabID(index))
			}
			offset += width + m.tabGap()
		}
		return nil
	}

	m.focusDetails()
	contentTop := tabY + 2 // tab bar and underline
	switch m.tab {
	case tabDatabases:
		row := m.databaseTableStart + y - (contentTop + 1 + m.databaseTableTop) // table header
		if y > contentTop+m.databaseTableTop && row >= 0 && row < len(m.databases) {
			m.databaseIndex = row
		}
	case tabAddons:
		row := m.addonTableStart + y - (contentTop + 1)
		if y > contentTop && row >= 0 && row < len(m.addons) {
			m.addonIndex = row
		}
	case tabConfig:
		if row := y - contentTop; row >= 0 && row < len(m.profileSettingRows()) {
			m.profileSettingsIndex = row
		}
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

func (m *Model) openConfirm(title, warning string, cancelModal modalMode, action func() tea.Cmd) {
	m.confirmForceToggle = false
	m.confirmTitle = title
	m.confirmPrompt = warning
	m.confirmCancelModal = cancelModal
	m.confirmAction = action
	m.modal = modalConfirm
}

func (m *Model) submitConfirm() tea.Cmd {
	action := m.confirmAction
	m.modal = modalNone
	m.confirmAction = nil
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

// prepareRestoreAction only needs a profile: a restore creates its database from
// a dump file, so it must also work when the profile has no databases yet.
func (m *Model) prepareRestoreAction() bool {
	if m.selectedProfileName() == "" {
		return false
	}
	m.taskProfileName = m.selectedProfileName()
	m.taskDatabaseName = ""
	m.taskDatabasePhysical = ""
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
	request := taskRequest{
		Kind:             taskDrop,
		ProfileName:      m.taskProfileName,
		DatabaseName:     m.taskDatabaseName,
		DatabasePhysical: m.taskDatabasePhysical,
		Yes:              true,
	}
	m.openConfirm(
		"Delete database "+databaseLabel(odoo.Database{Logical: m.taskDatabaseName, Physical: m.taskDatabasePhysical})+"?",
		"This permanently deletes the database and its filestore.",
		modalNone,
		func() tea.Cmd { return m.queueTask(request) },
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
	if !m.prepareRestoreAction() || m.rejectMutatingAction() {
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
	if !m.prepareRestoreAction() || m.rejectMutatingAction() {
		return nil
	}
	dir, err := os.Getwd()
	if err != nil || dir == "" {
		dir = "."
	}
	m.filePickerDirectory = false
	return m.openFilePicker(dir)
}

func (m *Model) openFilePicker(path string) tea.Cmd {
	m.filePickerRequest++
	m.filePickerPath = path
	m.filePickerItems = nil
	m.filePickerFilter = listFilter{}
	m.filePickerIndex = 0
	m.filePickerErr = nil
	m.filePickerPhase = phaseLoading
	m.modal = modalFilePicker
	return LoadDirectoryCmd(path, m.filePickerRequest)
}

func (m *Model) chooseFilePickerEntry() tea.Cmd {
	if m.filePickerPhase != phaseReady || !containsIndex(m.filePickerIndices(), m.filePickerIndex) {
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
	if addon := m.selectedAddon(); addon != nil {
		return m.openWorktreeBranchSelect(addon.Name)
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
	m.worktreeAddonName = source + "-" + strings.ReplaceAll(branch, "/", "-")
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
	if addon := m.selectedAddon(); addon != nil {
		m.openRemoveAddonConfirm(addon.Name)
	}
	return nil
}

func (m *Model) openRemoveAddonConfirm(name string) {
	m.removeAddonForce = false
	m.openConfirm(
		"Remove add-on "+name+"?",
		"This removes the checkout/worktree; attached profiles and child worktrees block removal.",
		modalNone,
		func() tea.Cmd {
			return m.queueTask(taskRequest{
				Kind: taskRemoveAddon, AddonName: name, Force: m.removeAddonForce, Yes: true,
			})
		},
	)
	m.confirmForceToggle = true
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

func (m *Model) currentSelectRequest(requestID uint64) bool {
	return requestID == m.selectRequest && m.modal == modalSelect
}

func (m *Model) closeSelect() {
	m.selectRequest++
	m.selectKind = selectNone
	m.selectItems = nil
	m.selectFilter = listFilter{}
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
	selectFirstIndex(&m.selectIndex, m.selectIndices())
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
	selectFirstIndex(&m.selectIndex, m.selectIndices())
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
	selectFirstIndex(&m.selectIndex, m.selectIndices())
	m.selectErr = nil
	m.selectPhase = phaseReady
}

func (m *Model) applySelectChoice() tea.Cmd {
	if m.selectPhase != phaseReady || !containsIndex(m.selectIndices(), m.selectIndex) {
		return nil
	}
	choice := m.selectItems[m.selectIndex]
	m.selectFilter = listFilter{}
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
	m.selectKind = selectNone
	m.selectItems = nil
	m.selectFilter = listFilter{}
	m.selectErr = nil
	m.selectPhase = phaseLoading
	m.openCreateProfileForm("")
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

func (m *Model) cycleCreateVersion(delta int) {
	count := len(m.selectItems)
	if count == 0 {
		return
	}
	index := -1
	for i, item := range m.selectItems {
		if item.Value == m.createProfileVersion || (item.Value == selectCustomValue && m.createProfileVersion == "") {
			index = i
			break
		}
	}
	if index < 0 {
		if delta > 0 {
			index = 0
		} else {
			index = count - 1
		}
	} else {
		index = (index + delta + count) % count
	}
	m.createProfileVersion = m.selectItems[index].Value
	if m.createProfileVersion == selectCustomValue {
		m.createProfileVersion = ""
	}
}

func (m *Model) submitCreateProfile() tea.Cmd {
	version := strings.TrimSpace(m.createProfileVersion)
	if version == "" {
		m.createFormErr = errors.New("Odoo version is required")
		return nil
	}
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
	return m.queueTask(taskRequest{Kind: taskCreate, ProfileName: name, Version: version})
}

func (m *Model) applyLoadedProfileConfig(config profile.Config) {
	m.configDBFilterMode = config.EffectiveDBFilterMode()
	m.configDBFilterPattern = config.DBFilterPattern
	m.configAdminPasswd = config.AdminPasswd
	m.configPhase = phaseReady
	m.configErr = nil
	rows := m.profileSettingRows()
	m.profileSettingsIndex = min(m.profileSettingsIndex, len(rows)-1)
}

// beginConfigLoad reads the profile settings shown on the Config tab.
func (m *Model) beginConfigLoad(reset bool) tea.Cmd {
	profileName := m.selectedProfileName()
	m.configRequest++
	if reset {
		m.configPhase = phaseLoading
		m.configErr = nil
		m.configFormErr = nil
		m.profileSettingsIndex = 0
	}
	if profileName == "" {
		m.configPhase = phaseEmpty
		return nil
	}
	return LoadProfileConfigCmd(m.ctx, m.service, profileName, m.configRequest)
}

type profileSettingRow uint8

const (
	settingDBFilterMode profileSettingRow = iota
	settingDBFilterPattern
	settingAdminPasswd
	settingLANPort
)

// profileSettingRows lists the settings that apply to the current mode. The
// pattern row only exists in custom mode, so the operator never edits a field
// that would be rejected.
func (m *Model) profileSettingRows() []profileSettingRow {
	rows := []profileSettingRow{settingDBFilterMode}
	if m.configDBFilterMode == profile.DBFilterModeCustom {
		rows = append(rows, settingDBFilterPattern)
	}
	return append(rows, settingAdminPasswd, settingLANPort)
}

func (m *Model) moveProfileSetting(delta int) {
	m.moveIndex(&m.profileSettingsIndex, len(m.profileSettingRows()), delta)
}

func (m *Model) editProfileSetting() tea.Cmd {
	if m.configPhase != phaseReady || m.selectedProfileName() == "" || m.rejectMutatingAction() {
		return nil
	}
	m.configProfileName = m.selectedProfileName()
	m.configFormErr = nil
	switch m.currentSettingRow() {
	case settingDBFilterMode:
		return m.applySelectedConfigMode(m.nextConfigMode())
	case settingDBFilterPattern:
		m.configPatternDraft = m.configDBFilterPattern
		m.formField = 0
		m.modal = modalConfigPattern
	case settingAdminPasswd:
		m.configPasswordDraft = m.configAdminPasswd
		m.formField = 0
		m.modal = modalConfigPassword
	case settingLANPort:
		m.configLANPortDraft = strconv.Itoa(m.profileDetail.LANPort)
		m.formField = 0
		m.modal = modalConfigLANPort
	}
	return nil
}

// nextConfigMode is the database filter mode that follows the current one.
func (m *Model) nextConfigMode() string {
	modes := []string{profile.DBFilterModeProfile, profile.DBFilterModeDisabled, profile.DBFilterModeCustom}
	for index, mode := range modes {
		if mode == m.configDBFilterMode {
			return modes[(index+1)%len(modes)]
		}
	}
	return modes[0]
}

// applySelectedConfigMode persists only the filter mode, except when custom is
// chosen without a pattern: then the pattern editor opens first so the change
// stays valid.
func (m *Model) applySelectedConfigMode(mode string) tea.Cmd {
	if mode == profile.DBFilterModeCustom && strings.TrimSpace(m.configDBFilterPattern) == "" {
		m.configPatternDraft = ""
		m.formField = 0
		m.modal = modalConfigPattern
		return nil
	}
	selected := mode
	m.configDBFilterMode = mode
	m.modal = modalNone
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
	m.modal = modalNone
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
	m.modal = modalNone
	return m.queueTask(taskRequest{
		Kind:         taskUpdateConfig,
		ProfileName:  m.configProfileName,
		ConfigUpdate: &profile.ConfigUpdate{AdminPasswd: &password},
	})
}

func (m *Model) submitConfigLANPort() tea.Cmd {
	port, err := strconv.Atoi(strings.TrimSpace(m.configLANPortDraft))
	if err != nil {
		m.configFormErr = errors.New("LAN port must be a number between 0 and 65535")
		return nil
	}
	update := profile.ConfigUpdate{LANPort: &port}
	if err := update.Validate(); err != nil {
		m.configFormErr = err
		return nil
	}
	m.modal = modalNone
	return m.queueTask(taskRequest{
		Kind:         taskUpdateConfig,
		ProfileName:  m.configProfileName,
		ConfigUpdate: &update,
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
	m.loading = len(m.profiles) == 0
	m.err = nil
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) updateModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalKeys:
		m.modal = modalNone
	case modalConfirm:
		if msg.Paste {
			return m, nil
		}
		switch msg.String() {
		case "esc", "n", "N":
			m.modal = m.confirmCancelModal
			m.confirmAction = nil
		case "enter", "y", "Y":
			return m, m.submitConfirm()
		case "f":
			if m.confirmForceToggle {
				m.removeAddonForce = !m.removeAddonForce
			}
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
			if value := keyInsertText(msg); value != "" {
				m.versionInput += value
			}
		}
	case modalFilePicker:
		switch msg.String() {
		case "esc":
			m.modal = modalNone
			if m.filePickerDirectory {
				m.modal = modalBackupDatabase
			}
		case "up":
			moveFilteredIndex(&m.filePickerIndex, m.filePickerIndices(), -1)
		case "down":
			moveFilteredIndex(&m.filePickerIndex, m.filePickerIndices(), 1)
		case "ctrl+h", "left":
			if m.filePickerFilter.query == "" {
				return m, m.filePickerParent()
			}
		case "enter":
			return m, m.chooseFilePickerEntry()
		case "ctrl+s":
			if m.filePickerDirectory && m.filePickerPhase != phaseLoading && m.filePickerPhase != phaseError {
				m.chooseBackupDirectory()
			}
		default:
			if m.filePickerFilter.update(msg) {
				selectFirstIndex(&m.filePickerIndex, m.filePickerIndices())
			}
		}
	case modalSelect:
		switch msg.String() {
		case "esc":
			m.closeSelect()
		case "up":
			moveFilteredIndex(&m.selectIndex, m.selectIndices(), -1)
		case "down":
			moveFilteredIndex(&m.selectIndex, m.selectIndices(), 1)
		case "enter":
			return m, m.applySelectChoice()
		default:
			if m.selectFilter.update(msg) {
				selectFirstIndex(&m.selectIndex, m.selectIndices())
			}
		}
	case modalInitDatabase, modalBackupDatabase, modalRestoreDatabase:
		if m.modal == modalBackupDatabase && msg.String() == "ctrl+o" {
			return m, m.startBackupDirectoryPicker()
		}
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
			m.appendFormText(keyInsertText(msg))
		}
	case modalCloneAddon, modalWorktreeAddon:
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
			if value := keyInsertText(msg); value != "" {
				m.appendFormText(value)
				m.addonFormErr = nil
			}
		}
	case modalConfigPattern, modalConfigPassword, modalConfigLANPort:
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
			if value := keyInsertText(msg); value != "" {
				m.appendFormText(value)
				m.configFormErr = nil
			}
		}
	case modalCreateProfile, modalAdminPassword:
		if m.modal == modalCreateProfile && m.formField == 1 && (msg.String() == "left" || msg.String() == "right") {
			delta := 1
			if msg.String() == "left" {
				delta = -1
			}
			m.cycleCreateVersion(delta)
			m.clearProfileFormErr()
			return m, nil
		}
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
			if value := keyInsertText(msg); value != "" {
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
		request := taskRequest{
			Kind:        taskWorktreeAddon,
			AddonName:   name,
			AddonSource: source,
			AddonBranch: branch,
			Yes:         true,
		}
		m.modal = modalNone
		return m.queueTask(request)
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
	case modalConfigLANPort:
		return m.submitConfigLANPort()
	case modalAdminPassword:
		return m.submitAdminPasswordForm()
	}
	return nil
}

func (m *Model) clearProfileFormErr() {
	switch m.modal {
	case modalCreateProfile:
		m.createFormErr = nil
	case modalConfigPattern, modalConfigPassword, modalConfigLANPort:
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
			m.openConfirm(
				"Restore database "+destination+"?",
				"Move or overwrite can replace existing database data.",
				modalNone,
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
		return 2
	case modalInitDatabase:
		return 2
	case modalBackupDatabase:
		return 4
	case modalRestoreDatabase:
		return 6
	case modalCreateProfile:
		return 2
	case modalConfigPattern, modalConfigPassword, modalConfigLANPort:
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
			return &m.worktreeAddonName
		case 1:
			return &m.worktreeAddonBranch
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
		switch m.formField {
		case 0:
			return &m.createProfileName
		case 1:
			return &m.createProfileVersion
		}
	case modalConfigPattern:
		if m.formField == 0 {
			return &m.configPatternDraft
		}
	case modalConfigPassword:
		if m.formField == 0 {
			return &m.configPasswordDraft
		}
	case modalConfigLANPort:
		if m.formField == 0 {
			return &m.configLANPortDraft
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

// pasteTarget is the text field a paste must land in, or nil when the current
// view has no text input. Restore and clone forms are plain single-line text,
// so a paste is just appended at the cursor (always at the end of the value).
func (m *Model) pasteTarget() *string {
	if m.logViewerOpen && m.logViewerTyping {
		return &m.logViewerSearch
	}
	if m.modal == modalNone {
		return nil
	}
	if target := m.formText(); target != nil {
		return target
	}
	switch m.modal {
	case modalRunVersion:
		return &m.versionInput
	case modalSelect:
		return &m.selectFilter.query
	case modalFilePicker:
		return &m.filePickerFilter.query
	}
	return nil
}

func (m *Model) applyClipboardPaste(msg ClipboardContentMsg) {
	if msg.Err != nil {
		m.notice = "paste failed: " + msg.Err.Error()
		return
	}
	target := m.pasteTarget()
	if target == nil || msg.Text == "" {
		return
	}
	*target += msg.Text
	if m.modal == modalSelect {
		selectFirstIndex(&m.selectIndex, m.selectIndices())
	}
	if m.modal == modalFilePicker {
		selectFirstIndex(&m.filePickerIndex, m.filePickerIndices())
	}
	m.notice = ""
	m.addonFormErr = nil
	m.configFormErr = nil
	m.restoreErr = nil
	m.adminPasswordErr = nil
	m.clearProfileFormErr()
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
	m.taskAddonNames = append([]string(nil), request.AddonNames...)
	m.taskAddonRecreate = request.Recreate
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
	m.taskStartedAt = time.Now()
	m.taskErr = nil
	m.taskNotice = ""
	m.taskOutput = ""
	m.taskExpanded = false
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
	m.taskAddonNames = nil
	m.taskAddonRecreate = false
	m.taskStarting = false
	m.taskRunning = false
	m.taskNotice = ""
	m.taskStatus = status
	m.taskExpanded = false
	m.taskCancelRequested = false
	m.taskErr = taskErr
}

func (m *Model) retainInteractiveOutput(name string, taskErr error) {
	m.taskName = name
	m.taskExpanded = false
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

func (m *Model) refreshAfterAddonMountTask(profileName string, attach bool, addonNames []string) tea.Cmd {
	if profileName == "" {
		return m.refreshAfterTask(profileName)
	}
	m.updateAddonAttachments(profileName, addonNames, attach)
	m.profileListOnlyRefresh = true
	m.loading = len(m.profiles) == 0
	m.err = nil
	return LoadProfilesCmd(m.ctx, m.service)
}

func (m *Model) updateAddonAttachments(profileName string, addonNames []string, attach bool) {
	selected := make(map[string]bool, len(addonNames))
	for _, name := range addonNames {
		selected[name] = true
	}
	for index := range m.addons {
		addon := &m.addons[index]
		if !selected[addon.Name] {
			continue
		}
		profiles := make([]string, 0, len(addon.AttachedProfiles)+1)
		found := false
		for _, name := range addon.AttachedProfiles {
			if name == profileName {
				found = true
				if !attach {
					continue
				}
			}
			profiles = append(profiles, name)
		}
		if attach && !found {
			profiles = append(profiles, profileName)
			sort.Strings(profiles)
		}
		addon.AttachedProfiles = profiles
	}
	if m.profileDetail.Name == profileName {
		attached := append([]string(nil), m.profileDetail.AttachedAddons...)
		for _, name := range addonNames {
			found := false
			for _, current := range attached {
				if current == name {
					found = true
					break
				}
			}
			if attach && !found {
				attached = append(attached, name)
			} else if !attach && found {
				filtered := attached[:0]
				for _, current := range attached {
					if current != name {
						filtered = append(filtered, current)
					}
				}
				attached = filtered
			}
		}
		m.profileDetail.AttachedAddons = attached
	}
}

func (m *Model) refreshAfterTask(profileName string) tea.Cmd {
	if profileName != "" {
		m.taskProfileName = profileName
	}
	m.profileListOnlyRefresh = false
	m.loading = len(m.profiles) == 0
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
		if profile.Name == m.profileDetail.Name {
			m.profileDetail.PendingRecreation = profile.PendingRecreation
		}
		if profile.Name == selected {
			m.profileIndex = index
		}
	}
	if len(m.profiles) == 0 {
		m.focusProfileList()
	}
}

func (m *Model) refreshProfileResources() tea.Cmd {
	profileName := m.selectedProfileName()
	if profileName == "" {
		return m.beginProfileResources()
	}
	m.profileRequest++
	m.databaseRequest++
	m.profileResourceName = profileName
	m.pendingDatabaseRefresh = false
	commands := []tea.Cmd{
		LoadProfileDetailCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadDatabasesCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadAddonsCmd(m.ctx, m.service, profileName, m.profileRequest),
	}
	commands = append(commands, m.beginConfigLoad(false))
	if m.tab == tabLogs {
		commands = append(commands, m.refreshLogLoad())
	}
	return tea.Batch(commands...)
}

func (m *Model) refreshLogLoad() tea.Cmd {
	preserveReadyView := m.logPhase == phaseReady
	cmd := m.beginLogLoad()
	if preserveReadyView && m.profileIsRunning() {
		m.logPhase = phaseReady
	}
	return cmd
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
	m.databaseInfos = nil
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
		m.beginConfigLoad(true)
		return nil
	}
	commands := []tea.Cmd{
		LoadProfileDetailCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadDatabasesCmd(m.ctx, m.service, profileName, m.profileRequest),
		LoadAddonsCmd(m.ctx, m.service, profileName, m.profileRequest),
		m.beginConfigLoad(true),
	}
	if m.tab == tabLogs {
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
	}
	commands := []tea.Cmd{LoadProfileDetailCmd(m.ctx, m.service, invalidation.ProfileName, m.profileRequest)}
	if m.tab == tabLogs {
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

func (m *Model) beginDatabaseInfosLoad() tea.Cmd {
	m.databaseRequest++
	if m.databaseInfoPhase != phaseReady {
		m.databaseInfoPhase = phaseLoading
		m.databaseInfos = make(map[string]odoo.DatabaseInfo, len(m.databases))
	}
	if len(m.databases) == 0 {
		m.databaseInfoPhase = phaseEmpty
		return nil
	}
	return LoadDatabaseInfosCmd(m.ctx, m.service, m.selectedProfileName(), append([]odoo.Database(nil), m.databases...), m.databaseRequest)
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

func addonAttached(addon addons.AddonStatus, profileName string) bool {
	for _, attachedProfile := range addon.AttachedProfiles {
		if attachedProfile == profileName {
			return true
		}
	}
	return false
}

func (m *Model) profileIsRunning() bool {
	profile := m.selectedProfile()
	return profile != nil && strings.EqualFold(profile.State, "running")
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

func (m *Model) logContent() string {
	if m.selectedProfile() != nil && !m.profileIsRunning() {
		return mutedStyle.Render("start the container to see logs")
	}

	entries := filterLogEntriesByDatabase(containerLogEntries(m.displayLogEntries(m.selectedProfileName())), m.logDatabase)
	output := renderLogEntries(entries, false)
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
			if value := keyInsertText(msg); value != "" {
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
	width := max(m.width, 40)
	height := max(m.height, 12)

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
	header := " " + titleStyle.Render("Logs · "+m.logViewerProfile) +
		"   " + mutedStyle.Render("source") + " " + secondStyle.Render(sourceLabel) +
		"   " + mutedStyle.Render("level") + " " + secondStyle.Render(levelLabel) +
		"   " + mutedStyle.Render("db") + " " + secondStyle.Render(database) +
		"   " + mutedStyle.Render("search") + " " + secondStyle.Render(search)

	entries := m.filteredLogEntries(m.logViewerProfile)
	content := renderLogEntries(entries, m.logViewerSource == logSourceAll)
	if content == "" {
		content = mutedStyle.Render("no log lines match the current filters")
	}

	wasBottom := !m.logViewerReady || m.logViewerViewport.AtBottom()
	m.logViewerViewport.Width = width - 2
	m.logViewerViewport.Height = max(3, height-3) // header, rule and footer
	m.logViewerViewport.SetContent(content)
	if m.logViewerFollow || wasBottom {
		m.logViewerViewport.GotoBottom()
	}
	m.logViewerReady = true

	body := strings.Split(m.logViewerViewport.View(), "\n")
	for index, line := range body {
		body[index] = " " + line
	}
	footer := hintsLine(width, []keyHint{
		{"esc", "close"}, {"tab", "source"}, {"d", "db"}, {"/", "search"}, {"1-4", "level"},
		{"f", "follow"}, {"g/G", "top/bottom"}, {"↑↓", "scroll"},
	})
	return header + "\n" + ruleStyle.Render(strings.Repeat("─", width)) + "\n" + strings.Join(body, "\n") + "\n" + footer
}

func (m *Model) logContainer(width, height int, content string) string {
	wasBottom := !m.logViewportReady || m.logViewport.AtBottom()
	m.logViewport.Width = width
	m.logViewport.Height = max(1, height)
	m.logViewport.SetContent(content)
	if wasBottom {
		m.logViewport.GotoBottom()
	}
	m.logViewportReady = true
	return m.logViewport.View()
}

// taskOutputTail renders the last useful output lines, indented under the task
// header and readable as plain text instead of dimmed noise.
func taskOutputTail(output string, maxLines int) (string, bool) {
	lines := usefulTaskLines(output)
	if len(lines) == 0 {
		return "", false
	}
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[len(lines)-maxLines:]
	}
	for index, line := range lines {
		lines[index] = "  " + strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n"), truncated
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

// modalKeyHints renders "key description" pairs separated by two spaces: the
// key in bold body text, the description muted.
func modalKeyHints(keys string) string {
	hints := make([]string, 0, 8)
	for _, part := range strings.Split(keys, "  ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, desc, _ := strings.Cut(part, " ")
		hints = append(hints, keyHint{key, desc}.render())
	}
	return strings.Join(hints, "  ")
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

func (m *Model) profileSettingValue(row profileSettingRow) (string, string, lipgloss.Style) {
	switch row {
	case settingDBFilterMode:
		return "Database filter", m.configDBFilterMode, textStyle
	case settingDBFilterPattern:
		if strings.TrimSpace(m.configDBFilterPattern) == "" {
			return "Filter pattern", "(required)", warningStyle
		}
		return "Filter pattern", m.configDBFilterPattern, textStyle
	case settingAdminPasswd:
		if m.configAdminPasswd == "" {
			return "Master password", "(not set)", mutedStyle
		}
		return "Master password", "set", runningStyle
	case settingLANPort:
		if m.profileDetail.LANPort == 0 {
			return "LAN port", "disabled", mutedStyle
		}
		return "LAN port", strconv.Itoa(m.profileDetail.LANPort), textStyle
	}
	return "", "", textStyle
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
		return "available"
	}
	return "unavailable"
}

func errorText(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

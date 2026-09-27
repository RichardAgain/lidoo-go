package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/odoo"
	"lidoo/internal/profile"
)

func TestProfileFocusShowsLogsWithoutLogTab(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "running"}}
	model.profileLogs["testing"] = &profileLogBuffer{container: "profile log output"}
	model.logPhase = phaseReady

	view := ansi.Strip(model.rightView(60))
	if !strings.Contains(view, "profile log output") {
		t.Fatalf("profile focus should show logs, got %q", view)
	}
	if strings.Contains(view, "Profile details") {
		t.Fatalf("profile focus should not show profile details, got %q", view)
	}
	if strings.Contains(view, "Log") {
		t.Fatalf("log tab should be removed, got %q", view)
	}
}

func TestStoppedProfileShowsLogMessage(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "exited"}}
	model.profileLogs["testing"] = &profileLogBuffer{container: "stale log output"}
	model.logPhase = phaseReady

	const want = "start the container to see logs"
	if got := ansi.Strip(model.logContent()); got != want {
		t.Fatalf("log content = %q, want %q", got, want)
	}
	if cmd := model.beginLogLoad(); cmd != nil {
		t.Fatal("stopped profile should not start a log command")
	}
	if model.logPhase != phaseUnavailable {
		t.Fatalf("log phase = %v, want unavailable", model.logPhase)
	}
}

func TestVersionSelectIncludesCustomOption(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.setVersionSelectItems([]docker.VersionOption{
		{Version: "18", Image: true},
		{Version: "17"},
	})

	if len(model.selectItems) != 3 {
		t.Fatalf("select items = %d, want 3", len(model.selectItems))
	}
	if model.selectItems[2].Value != selectCustomValue {
		t.Fatalf("last item value = %q, want custom sentinel", model.selectItems[2].Value)
	}
	if model.selectItems[0].Label != "18" || model.selectItems[0].Hint != "image built" {
		t.Fatalf("first item = %+v", model.selectItems[0])
	}
	if model.selectPhase != phaseReady {
		t.Fatalf("select phase = %v, want ready", model.selectPhase)
	}
}

func TestCreateProfileVersionChoiceOpensForm(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.selectKind = selectCreateProfileVersion
	model.selectPhase = phaseReady
	model.selectItems = []selectItem{{Label: "18", Value: "18"}}
	model.selectIndex = 0

	if cmd := model.applySelectChoice(); cmd != nil {
		t.Fatal("choosing a version should not start a command")
	}
	if model.modal != modalCreateProfile {
		t.Fatalf("modal = %v, want create profile", model.modal)
	}
	if model.createProfileVersion != "18" {
		t.Fatalf("create version = %q, want 18", model.createProfileVersion)
	}
}

func TestCustomVersionChoiceOpensTextInput(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.selectKind = selectCreateProfileVersion
	model.selectPhase = phaseReady
	model.selectItems = []selectItem{{Label: "Custom version…", Value: selectCustomValue}}
	model.selectIndex = 0

	if cmd := model.applySelectChoice(); cmd != nil {
		t.Fatal("choosing custom version should not start a command")
	}
	if model.modal != modalRunVersion {
		t.Fatalf("modal = %v, want version input", model.modal)
	}
	if model.versionPurpose != versionForCreate {
		t.Fatalf("version purpose = %v, want create", model.versionPurpose)
	}
}

func TestCreateProfileRejectsDuplicateName(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo"}}
	model.modal = modalCreateProfile
	model.createProfileVersion = "18"
	model.createProfileName = "demo"

	if cmd := model.submitCreateProfile(); cmd != nil {
		t.Fatal("duplicate profile should not queue a task")
	}
	if model.createFormErr == nil {
		t.Fatal("duplicate profile should set a form error")
	}
}

func TestCreateProfileQueuesCreateWithoutRun(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCreateProfile
	model.createProfileVersion = "18"
	model.createProfileName = "fresh"

	cmd := model.submitCreateProfile()
	if cmd == nil {
		t.Fatal("valid profile should queue a create task")
	}
	if model.pendingTask == nil || model.pendingTask.Kind != taskCreate {
		t.Fatalf("pending task = %+v, want create", model.pendingTask)
	}
	if model.pendingTask.ProfileName != "fresh" || model.pendingTask.Version != "18" {
		t.Fatalf("pending task = %+v", model.pendingTask)
	}
	if model.pendingTask.Wait {
		t.Fatal("create should not wait for readiness")
	}
}

func TestProfileSettingRowsFollowMode(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalProfileSettings

	model.configDBFilterMode = profile.DBFilterModeProfile
	if got := len(model.profileSettingRows()); got != 2 {
		t.Fatalf("profile mode rows = %d, want 2", got)
	}
	model.configDBFilterMode = profile.DBFilterModeCustom
	rows := model.profileSettingRows()
	if len(rows) != 3 {
		t.Fatalf("custom mode rows = %d, want 3", len(rows))
	}
	if rows[1] != settingDBFilterPattern {
		t.Fatalf("second row = %v, want pattern", rows[1])
	}
}

func TestLoadedProfileConfigOpensSettings(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.applyLoadedProfileConfig(profile.Config{
		DBFilterMode:    profile.DBFilterModeDisabled,
		DBFilterPattern: "",
		AdminPasswd:     "master",
	})

	if model.modal != modalProfileSettings {
		t.Fatalf("modal = %v, want settings", model.modal)
	}
	if model.configDBFilterMode != profile.DBFilterModeDisabled {
		t.Fatalf("mode = %q", model.configDBFilterMode)
	}
	if model.configAdminPasswd != "master" {
		t.Fatalf("master password = %q", model.configAdminPasswd)
	}
}

func TestEditSettingOpensModeSelect(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalProfileSettings
	model.configProfileName = "demo"
	model.configDBFilterMode = profile.DBFilterModeDisabled
	model.profileSettingsIndex = 0

	if cmd := model.editProfileSetting(); cmd != nil {
		t.Fatal("editing the mode should not queue a task")
	}
	if model.modal != modalSelect {
		t.Fatalf("modal = %v, want select", model.modal)
	}
	if len(model.selectItems) != 3 {
		t.Fatalf("select items = %d, want 3", len(model.selectItems))
	}
	if model.selectItems[model.selectIndex].Value != profile.DBFilterModeDisabled {
		t.Fatalf("default index points at %q", model.selectItems[model.selectIndex].Value)
	}
}

func TestSelectCustomModeWithoutPatternOpensPatternEditor(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.configProfileName = "demo"
	model.configDBFilterMode = profile.DBFilterModeProfile
	model.configDBFilterPattern = ""

	if cmd := model.applySelectedConfigMode(profile.DBFilterModeCustom); cmd != nil {
		t.Fatal("custom without pattern should not queue a task yet")
	}
	if model.modal != modalConfigPattern {
		t.Fatalf("modal = %v, want pattern editor", model.modal)
	}
}

func TestSelectDisabledModeChangesOnlyTheMode(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.configProfileName = "demo"
	model.configDBFilterMode = profile.DBFilterModeProfile
	model.configAdminPasswd = "master"

	cmd := model.applySelectedConfigMode(profile.DBFilterModeDisabled)
	if cmd == nil || model.pendingTask == nil {
		t.Fatal("changing mode should queue a config task")
	}
	update := model.pendingTask.ConfigUpdate
	if update == nil || update.DBFilterMode == nil || *update.DBFilterMode != profile.DBFilterModeDisabled {
		t.Fatalf("update = %+v", update)
	}
	if update.DBFilterPattern != nil || update.AdminPasswd != nil {
		t.Fatalf("mode change should not touch other fields: %+v", update)
	}
}

func TestSubmitConfigPasswordChangesOnlyPassword(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalConfigPassword
	model.configProfileName = "demo"
	model.configPasswordDraft = "s3cret"

	cmd := model.submitConfigPassword()
	if cmd == nil || model.pendingTask == nil {
		t.Fatal("password change should queue a config task")
	}
	update := model.pendingTask.ConfigUpdate
	if update == nil || update.AdminPasswd == nil || *update.AdminPasswd != "s3cret" {
		t.Fatalf("update = %+v", update)
	}
	if update.DBFilterMode != nil || update.DBFilterPattern != nil {
		t.Fatalf("password change should not touch other fields: %+v", update)
	}
	if model.modal != modalProfileSettings {
		t.Fatalf("modal = %v, want back to settings", model.modal)
	}
}

func TestToggleProfileQueuesStopWhenRunning(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running"}}

	if cmd := model.toggleSelectedProfile(); cmd == nil {
		t.Fatal("toggle should queue a task")
	}
	if model.pendingTask == nil || model.pendingTask.Kind != taskStop {
		t.Fatalf("pending task = %+v, want stop", model.pendingTask)
	}
}

func TestToggleProfileQueuesRunWhenStopped(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "exited"}}

	if cmd := model.toggleSelectedProfile(); cmd == nil {
		t.Fatal("toggle should queue a task")
	}
	if model.pendingTask == nil || model.pendingTask.Kind != taskRun {
		t.Fatalf("pending task = %+v, want run", model.pendingTask)
	}
}

func TestAdminPasswordFormRejectsMismatch(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalAdminPassword
	model.taskProfileName = "demo"
	model.taskDatabaseName = "demo_db"
	model.adminPassword = "secret"
	model.adminPasswordConfirm = "other"

	if cmd := model.submitAdminPasswordForm(); cmd != nil {
		t.Fatal("mismatched passwords should not queue a task")
	}
	if model.adminPasswordErr == nil {
		t.Fatal("mismatched passwords should set an error")
	}
}

func TestAdminPasswordFormQueuesTask(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalAdminPassword
	model.taskProfileName = "demo"
	model.taskDatabaseName = "demo_db"
	model.taskDatabasePhysical = "demo__demo_db"
	model.adminPassword = "secret"
	model.adminPasswordConfirm = "secret"

	if cmd := model.submitAdminPasswordForm(); cmd == nil {
		t.Fatal("valid passwords should queue a task")
	}
	if model.pendingTask == nil || model.pendingTask.Kind != taskSetAdminPassword {
		t.Fatalf("pending task = %+v", model.pendingTask)
	}
	if model.pendingTask.Password != "secret" || model.pendingTask.DatabaseName != "demo_db" {
		t.Fatalf("pending task = %+v", model.pendingTask)
	}
}

func TestLogBufferSeparatesContainerAndTasks(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.appendContainerLog("demo", "container line\n")
	model.appendTaskLog("demo", "task line\n")

	entries := model.logEntries("demo")
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Source != logSourceContainer || entries[1].Source != logSourceTask {
		t.Fatalf("sources = %v, %v", entries[0].Source, entries[1].Source)
	}
}

func TestViewsRenderWithoutPanic(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width = 120
	model.height = 40
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running", URL: "http://demo.lidoo.localhost"}}
	model.databases = []odoo.Database{{Logical: "demo_db", Physical: "demo__demo_db"}}
	model.databasePhase = phaseReady
	model.databaseInfo = odoo.DatabaseInfo{Logical: "demo_db", Physical: "demo__demo_db", Size: "1 MB", Available: true}
	model.databaseInfoPhase = phaseReady
	model.appendContainerLog("demo", "2026-01-02 03:04:05,678 9 INFO demo__demo_db odoo.modules.loading: ready 1 0.100 0.200\n")
	model.appendTaskLog("demo", "done\n")
	model.logPhase = phaseReady
	model.configProfileName = "demo"
	model.configDBFilterMode = profile.DBFilterModeCustom
	model.configDBFilterPattern = "^demo__.*$"
	model.configAdminPasswd = "master"

	_ = model.View()
	_ = model.leftView(40)
	_ = model.rightView(80)
	_ = model.smallView()
	_ = model.databaseTabView()

	for _, modal := range []modalMode{modalProfileSettings, modalConfigPattern, modalConfigPassword, modalAdminPassword, modalSelect, modalCreateProfile, modalFilePicker} {
		model.modal = modal
		_ = model.modalView()
	}

	model.openLogViewer()
	_ = model.logViewerView()
}

func TestLogViewerDatabaseFilter(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.appendContainerLog("demo", "2026-01-02 03:04:05,678 9 INFO demo__a odoo.modules.loading: first 1 0.1 0.1\n2026-01-02 03:04:06,678 9 INFO demo__b odoo.modules.loading: second 1 0.1 0.1\n")
	model.logViewerProfile = "demo"
	model.logViewerSource = logSourceAll
	model.logViewerLevel = logLevelFilterAll

	model.logViewerDatabase = "demo__a"
	filtered := model.filteredLogEntries("demo")
	if len(filtered) != 1 || filtered[0].Database != "demo__a" {
		t.Fatalf("filtered = %+v", filtered)
	}

	model.logViewerDatabase = ""
	if got := len(model.filteredLogEntries("demo")); got != 2 {
		t.Fatalf("unfiltered = %d, want 2", got)
	}
}

func TestTypedConfirmRequiresExactName(t *testing.T) {
	model := NewModel(context.Background(), nil)
	queued := false
	model.openTypedConfirm("irreversible", "demo", func() tea.Cmd {
		queued = true
		return nil
	})

	model.confirmInput = "dem"
	if cmd := model.submitTypedConfirm(); cmd != nil {
		t.Fatal("a wrong name should not proceed")
	}
	if model.confirmErr == nil || queued {
		t.Fatal("wrong name should error and not run the action")
	}

	model.confirmInput = "demo"
	model.submitTypedConfirm()
	if !queued {
		t.Fatal("exact name should run the action")
	}
	if model.modal != modalNone {
		t.Fatalf("modal = %v, want none", model.modal)
	}
}

func TestRunToggleWaitsForReadiness(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "exited"}}

	model.toggleSelectedProfile()
	if model.pendingTask == nil || model.pendingTask.Kind != taskRun || !model.pendingTask.Wait {
		t.Fatalf("pending task = %+v, want run with wait", model.pendingTask)
	}
}

func TestFilePickerSelectsDump(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.focus = focusDatabases
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running"}}
	model.databases = []odoo.Database{{Logical: "demo_db", Physical: "demo__demo_db"}}
	model.databaseIndex = 0
	model.modal = modalFilePicker
	model.filePickerPhase = phaseReady
	model.filePickerItems = []files.DirEntry{{Name: "a.zip", Path: "/tmp/a.zip"}}
	model.filePickerIndex = 0

	if cmd := model.chooseFilePickerEntry(); cmd != nil {
		t.Fatal("choosing a file should not queue a task")
	}
	if model.modal != modalRestoreDatabase {
		t.Fatalf("modal = %v, want restore form", model.modal)
	}
	if model.restoreSource != "/tmp/a.zip" {
		t.Fatalf("source = %q", model.restoreSource)
	}
}

func TestLogDatabaseSelectItemsIncludeAll(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running"}}
	model.databases = []odoo.Database{{Logical: "demo_db", Physical: "demo__demo_db"}}
	model.appendContainerLog("demo", "2026-01-02 03:04:05,678 9 INFO other odoo.x: hi 1 0.1 0.1\n")

	items := model.logDatabaseSelectItems()
	if len(items) != 3 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Value != "" {
		t.Fatalf("first item = %+v, want all", items[0])
	}
	if items[1].Value != "demo__demo_db" || items[2].Value != "other" {
		t.Fatalf("items = %+v", items)
	}
}

func TestProfileIndexAtRow(t *testing.T) {
	if got := profileIndexAtRow(2, 3); got != 0 {
		t.Fatalf("row 2 = %d, want 0", got)
	}
	if got := profileIndexAtRow(8, 3); got != 1 {
		t.Fatalf("row 8 = %d, want 1", got)
	}
	if got := profileIndexAtRow(100, 3); got != 2 {
		t.Fatalf("row 100 = %d, want 2 (clamped)", got)
	}
	if got := profileIndexAtRow(0, 0); got != -1 {
		t.Fatalf("no profiles = %d, want -1", got)
	}
}

func TestRestoreDefaultDestinationIsFresh(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.focus = focusDatabases
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running"}}
	model.databases = []odoo.Database{{Logical: "demo_db", Physical: "demo__demo_db"}}
	model.databaseIndex = 0

	model.openRestoreForm("/home/me/Downloads/IPM Backup.zip")
	if model.modal != modalRestoreDatabase {
		t.Fatalf("modal = %v, want restore form", model.modal)
	}
	if model.restoreDestination != "ipm_backup" {
		t.Fatalf("destination = %q, want ipm_backup", model.restoreDestination)
	}
}

func TestRestoreDefaultDestinationAvoidsCollision(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running"}}
	model.databases = []odoo.Database{{Logical: "ipm_backup", Physical: "pp__ipm_backup"}}

	if got := model.defaultRestoreDestination("/tmp/ipm_backup.zip"); got != "ipm_backup_restore" {
		t.Fatalf("destination = %q, want ipm_backup_restore", got)
	}
}

func TestRestoreCopyRejectsExistingDestination(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalRestoreDatabase
	model.taskProfileName = "demo"
	model.databases = []odoo.Database{{Logical: "demo_db", Physical: "demo__demo_db"}}
	model.restoreSource = "/tmp/x.zip"
	model.restoreDestination = "demo_db"
	model.restoreCopy = true
	model.restoreForce = false

	if cmd := model.submitDatabaseForm(); cmd != nil {
		t.Fatal("existing destination in copy mode should not queue a task")
	}
	if model.restoreErr == nil {
		t.Fatal("existing destination in copy mode should set an error")
	}
}

func TestCtrlCQuitsDuringTask(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.taskRunning = true

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c command = %T, want tea.QuitMsg", cmd())
	}
}

func TestCtrlCQuitsWithModalOpen(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalTypedConfirm

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should quit even with a modal open")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c command = %T, want tea.QuitMsg", cmd())
	}
}

func TestCKeyCancelsTaskWithoutQuitting(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.taskRunning = true

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd != nil {
		t.Fatal("c should not quit")
	}
	if !model.taskCancelRequested {
		t.Fatal("c should request task cancellation")
	}
}

func TestEmptyStateWhenNoProfiles(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.loading = false
	view := ansi.Strip(model.rightView(60))
	if !strings.Contains(view, "create your first container") {
		t.Fatalf("empty state = %q", view)
	}
}

func TestMoveFocusStaysOnProfilesWhenEmpty(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.focus = focusDatabases

	model.moveFocus(1)
	if model.focus != focusProfiles {
		t.Fatalf("focus = %v, want profiles", model.focus)
	}
}

func TestModalTitleSeparatorAddsRule(t *testing.T) {
	lines := modalTitleSeparator([]string{"Title", "body"}, 40)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if !strings.Contains(lines[1], "─") {
		t.Fatalf("separator = %q", lines[1])
	}
	if len(modalTitleSeparator([]string{"only"}, 40)) != 1 {
		t.Fatal("a single line should be unchanged")
	}
}

func TestFitViewClampsWidthAndHeight(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width = 20
	model.height = 3

	view := model.fitView("a very long line that overflows\nline2\nline3\nline4\n")
	lines := strings.Split(view, "\n")
	if len(lines) > 3 {
		t.Fatalf("lines = %d, want at most 3", len(lines))
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 20 {
			t.Fatalf("line %q is wider than 20", line)
		}
	}
}

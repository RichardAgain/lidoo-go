package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/odoo"
	"lidoo/internal/profile"
)

func TestAddonTableShowsLiveBranch(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.pane, model.tab = paneDetails, tabAddons
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "running"}}
	model.addonPhase = phaseReady
	model.addons = []addons.AddonStatus{
		{Name: "enterprise", Kind: "clone", Branch: "saas-18.1", Entry: addons.Entry{Source: "git@github.com:acme/enterprise.git"}},
	}

	rows := ansi.Strip(model.addonTabView(80, 20))
	if !strings.Contains(rows, "saas-18.1") {
		t.Fatalf("branch column should show the current branch, got %q", rows)
	}
	if strings.Contains(rows, "—") {
		t.Fatalf("branch column should not be empty, got %q", rows)
	}
}

func TestModalHidesBackgroundLogs(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width = 120
	model.height = 40
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "running", URL: "http://testing.lidoo.localhost"}}
	model.profileLogs["testing"] = &profileLogBuffer{container: "Actualizando archivos: 58%"}
	model.logPhase = phaseReady

	if view := ansi.Strip(model.View()); !strings.Contains(view, "Actualizando archivos: 58%") {
		t.Fatalf("logs should be visible without a modal, got %q", view)
	}

	model.modal = modalCloneAddon
	view := ansi.Strip(model.View())
	if strings.Contains(view, "Actualizando archivos: 58%") {
		t.Fatalf("background logs should be hidden behind a modal, got %q", view)
	}
	if !strings.Contains(view, "Clone and register add-on") {
		t.Fatalf("modal should be visible, got %q", view)
	}
}

func TestAddonTableMarksAttached(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.pane, model.tab = paneDetails, tabAddons
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "running"}}
	model.addonPhase = phaseReady
	model.addons = []addons.AddonStatus{
		{Name: "enterprise", Kind: "clone", Branch: "18.0", AttachedProfiles: []string{"testing"}},
		{Name: "lidoo", Kind: "clone", Branch: "18.0"},
	}

	rows := ansi.Strip(model.addonTabView(80, 20))
	if !strings.Contains(rows, "✓") {
		t.Fatalf("attached add-on should show a check, got %q", rows)
	}
}

func TestTaskOutputDropsPythonTracebackNoise(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.taskName = "update"
	model.taskStatus = "completed"
	model.taskExpanded = true
	model.taskOutput = strings.Join([]string{
		"loading addons",
		`/usr/lib/python3/odoo/tools/func.py:42: DeprecationWarning: call of deprecated method`,
		`  File "/usr/lib/python3/dist-packages/odoo/tools/func.py", line 42, in __get__`,
		"    value = self.fget(obj)",
		`  File "/usr/lib/python3/dist-packages/odoo/modules/registry.py", line 400, in field_computed`,
		"    warnings.warn(",
		"",
		`database "testing__ipm_2026-09-25_20-13-47" updated`,
	}, "\n")

	view := ansi.Strip(strings.Join(model.taskRegion(80, 20), "\n"))
	if !strings.Contains(view, `database "testing__ipm_2026-09-25_20-13-47" updated`) {
		t.Fatalf("the result line must be visible, got %q", view)
	}
	if strings.Contains(view, "File \"") || strings.Contains(view, "self.fget") || strings.Contains(view, "warnings.warn(") {
		t.Fatalf("traceback noise should be dropped, got %q", view)
	}
	if !strings.Contains(view, "loading addons") {
		t.Fatalf("real progress lines must be kept, got %q", view)
	}
}

func TestTaskOutputCollapsesCarriageReturnProgress(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.taskName = "clone add-on"
	model.taskStatus = "completed"
	model.taskExpanded = true
	model.taskOutput = "Enumerating objects: 10%\rEnumerating objects: 58%\rEnumerating objects: 100%\r"

	view := ansi.Strip(strings.Join(model.taskRegion(80, 20), "\n"))
	if !strings.Contains(view, "Enumerating objects: 100%") {
		t.Fatalf("progress should keep its last update, got %q", view)
	}
	if strings.Contains(view, "10%") || strings.Contains(view, "58%") {
		t.Fatalf("stale progress frames should be dropped, got %q", view)
	}
}

func TestTaskViewShowsRunningTitleAndTruncation(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.taskName = "update"
	model.taskStatus = "running"
	model.taskRunning = true
	for index := 0; index < 14; index++ {
		model.taskOutput += fmt.Sprintf("step %d\n", index)
	}

	view := ansi.Strip(strings.Join(model.taskRegion(80, 20), "\n"))
	if !strings.Contains(view, "c cancel") {
		t.Fatalf("a running task should offer cancel, got %q", view)
	}
	if !strings.Contains(view, "older output above") {
		t.Fatalf("truncated output should be marked, got %q", view)
	}
	if !strings.Contains(view, "step 13") {
		t.Fatalf("the newest line must be visible, got %q", view)
	}
	if strings.Contains(view, "step 3\n") {
		t.Fatalf("only the tail should be shown, got %q", view)
	}
}

func TestProfileFocusShowsLogs(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "running"}}
	model.profileLogs["testing"] = &profileLogBuffer{container: "profile log output"}
	model.logPhase = phaseReady

	view := ansi.Strip(model.rightView(model.frame()))
	if !strings.Contains(view, "profile log output") {
		t.Fatalf("profile focus should show logs, got %q", view)
	}
	if strings.Contains(view, "Profile details") {
		t.Fatalf("profile focus should not show profile details, got %q", view)
	}
	if !strings.Contains(view, "1 Logs") {
		t.Fatalf("tab bar should list the Logs tab, got %q", view)
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
		{Version: "18", Dockerfile: true, Image: true},
		{Version: "17"},
	})

	if len(model.selectItems) != 3 {
		t.Fatalf("select items = %d, want 3", len(model.selectItems))
	}
	if model.selectItems[2].Value != selectCustomValue {
		t.Fatalf("last item value = %q, want custom sentinel", model.selectItems[2].Value)
	}
	if model.selectItems[0].Label != "18" || model.selectItems[0].Hint != "dockerfile · image built" {
		t.Fatalf("first item = %+v", model.selectItems[0])
	}
	if model.selectItems[1].Hint != "no dockerfile" {
		t.Fatalf("second item = %+v", model.selectItems[1])
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

	model.configDBFilterMode = profile.DBFilterModeProfile
	if got := len(model.profileSettingRows()); got != 3 {
		t.Fatalf("profile mode rows = %d, want 3", got)
	}
	model.configDBFilterMode = profile.DBFilterModeCustom
	rows := model.profileSettingRows()
	if len(rows) != 4 {
		t.Fatalf("custom mode rows = %d, want 4", len(rows))
	}
	if rows[1] != settingDBFilterPattern {
		t.Fatalf("second row = %v, want pattern", rows[1])
	}
}

func TestLoadedProfileConfigFillsConfigTab(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.applyLoadedProfileConfig(profile.Config{
		DBFilterMode:    profile.DBFilterModeDisabled,
		DBFilterPattern: "",
		AdminPasswd:     "master",
	})

	if model.modal != modalNone || model.configPhase != phaseReady {
		t.Fatalf("modal/phase = %v/%v, want none/ready", model.modal, model.configPhase)
	}
	if model.configDBFilterMode != profile.DBFilterModeDisabled {
		t.Fatalf("mode = %q", model.configDBFilterMode)
	}
	if model.configAdminPasswd != "master" {
		t.Fatalf("master password = %q", model.configAdminPasswd)
	}
}

func configModel() *Model {
	model := NewModel(context.Background(), nil)
	model.width, model.height = 100, 30
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running", Version: "18", URL: "http://demo.lidoo.localhost"}}
	model.profilePhase = phaseReady
	model.profileDetail = docker.ProfileDetail{Name: "demo", DockerState: "running", LANPort: 8070}
	model.configProfileName = "demo"
	model.applyLoadedProfileConfig(profile.Config{DBFilterMode: profile.DBFilterModeProfile})
	model.pane, model.tab = paneDetails, tabConfig
	return model
}

func TestConfigTabEnterCyclesFilterMode(t *testing.T) {
	model := configModel()
	if cmd := model.editProfileSetting(); cmd == nil {
		t.Fatal("cycling the mode should queue a config task")
	}
	update := model.pendingTask.ConfigUpdate
	if update == nil || update.DBFilterMode == nil || *update.DBFilterMode != profile.DBFilterModeDisabled {
		t.Fatalf("update = %+v, want disabled", update)
	}
}

func TestConfigTabRowsOpenEditForms(t *testing.T) {
	model := configModel()
	model.updateKey(keyMsg("j"))
	model.updateKey(keyMsg("enter"))
	if model.modal != modalConfigPassword {
		t.Fatalf("modal = %v, want master password form", model.modal)
	}
	model.modal = modalNone
	model.updateKey(keyMsg("j"))
	model.updateKey(keyMsg("enter"))
	if model.modal != modalConfigLANPort || model.configLANPortDraft != "8070" {
		t.Fatalf("modal/draft = %v/%q, want LAN port form with 8070", model.modal, model.configLANPortDraft)
	}
}

func TestEKeyJumpsToConfigTab(t *testing.T) {
	model := navModel()
	model.updateKey(keyMsg("e"))
	if model.pane != paneDetails || model.tab != tabConfig {
		t.Fatalf("pane/tab = %v/%v, want details/config", model.pane, model.tab)
	}
}

func TestConfigTabShowsRowsAndPendingRecreation(t *testing.T) {
	model := configModel()
	model.profileDetail.PendingRecreation = docker.RecreationStatus{Checked: true, Pending: true, Reason: "LAN port changed"}
	view := ansi.Strip(model.configTabView(80))
	for _, want := range []string{"Database filter", "Master password", "LAN port", "8070", "changes waiting", "R recreates the container"} {
		if !strings.Contains(view, want) {
			t.Fatalf("config tab missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Filter pattern") {
		t.Fatalf("pattern row should only exist in custom mode:\n%s", view)
	}
}

func TestCancelledCustomModeDoesNotChangeDisplayedMode(t *testing.T) {
	model := configModel()
	model.applySelectedConfigMode(profile.DBFilterModeCustom)
	if model.modal != modalConfigPattern {
		t.Fatalf("modal = %v, want pattern editor", model.modal)
	}
	if model.configDBFilterMode != profile.DBFilterModeProfile {
		t.Fatalf("mode = %q, want unchanged until saved", model.configDBFilterMode)
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
	if model.modal != modalNone {
		t.Fatalf("modal = %v, want closed", model.modal)
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
	model.databaseInfoPhase = phaseReady
	model.databaseInfos = map[string]odoo.DatabaseInfo{"demo__demo_db": {Logical: "demo_db", Physical: "demo__demo_db", Size: "1 MB", Available: true}}
	model.appendContainerLog("demo", "2026-01-02 03:04:05,678 9 INFO demo__demo_db odoo.modules.loading: ready 1 0.100 0.200\n")
	model.appendTaskLog("demo", "done\n")
	model.logPhase = phaseReady
	model.configProfileName = "demo"
	model.configDBFilterMode = profile.DBFilterModeCustom
	model.configDBFilterPattern = "^demo__.*$"
	model.configAdminPasswd = "master"

	_ = model.View()
	_ = model.leftView(model.frame())
	_ = model.rightView(model.frame())
	_ = model.smallView()
	_ = model.databaseTabView(80, 20)

	for _, modal := range []modalMode{modalConfigPattern, modalConfigPassword, modalAdminPassword, modalSelect, modalCreateProfile, modalFilePicker} {
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

func TestConfirmRunsActionOnSubmit(t *testing.T) {
	model := NewModel(context.Background(), nil)
	queued := false
	model.openConfirm("Drop demo?", "irreversible", modalNone, func() tea.Cmd {
		queued = true
		return nil
	})
	if model.modal != modalConfirm {
		t.Fatalf("modal = %v, want confirm", model.modal)
	}

	model.submitConfirm()
	if !queued {
		t.Fatal("submit should run the action")
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
	model.pane, model.tab = paneDetails, tabDatabases
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

func manyProfiles(count int) []docker.ProfileSummary {
	profiles := make([]docker.ProfileSummary, count)
	for index := range profiles {
		profiles[index] = docker.ProfileSummary{Name: fmt.Sprintf("profile-%02d", index), State: "exited", Version: "18"}
	}
	return profiles
}

func TestProfileListKeepsSelectionVisible(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width, model.height = 100, 24
	model.loading = false
	model.profiles = manyProfiles(40)
	for _, index := range []int{0, 7, 20, 39, 3} {
		model.profileIndex = index
		view := ansi.Strip(model.View())
		if !strings.Contains(view, fmt.Sprintf("▌ ○ profile-%02d", index)) {
			t.Fatalf("selection %d is not visible:\n%s", index, view)
		}
		if got := len(strings.Split(view, "\n")); got != 24 {
			t.Fatalf("view has %d lines, want 24", got)
		}
	}
}

func TestClickSelectsProfileRow(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width, model.height = 100, 24
	model.loading = false
	model.profiles = manyProfiles(5)
	lines := strings.Split(ansi.Strip(model.View()), "\n")
	row := -1
	for index, line := range lines {
		if strings.Contains(line, "profile-03") {
			row = index
		}
	}
	if row < 0 {
		t.Fatalf("profile-03 not rendered:\n%s", strings.Join(lines, "\n"))
	}
	model.handleLeftClick(5, row)
	if model.profileIndex != 3 {
		t.Fatalf("profileIndex = %d, want 3", model.profileIndex)
	}
}

func TestSinglePendingRecreationShowsInRowAndHeader(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width, model.height = 100, 24
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running", Version: "18", URL: "http://demo.lidoo.localhost", PendingRecreation: docker.RecreationStatus{Checked: true, Pending: true}}}
	view := ansi.Strip(model.View())
	if strings.Count(view, "↻") < 2 {
		t.Fatalf("pending recreation should show in the row and the header:\n%s", view)
	}
	if !strings.Contains(view, "http://demo.lidoo.localhost") {
		t.Fatalf("header should show the URL:\n%s", view)
	}
}

func TestRestoreDefaultDestinationIsFresh(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.pane, model.tab = paneDetails, tabDatabases
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

func TestRestoreWorksWithoutExistingDatabases(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.pane, model.tab = paneDetails, tabDatabases
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running"}}
	model.databases = nil
	model.databasePhase = phaseEmpty

	updated, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model = updated.(*Model)
	if model.modal != modalFilePicker {
		t.Fatalf("modal = %v, want file picker", model.modal)
	}
	if cmd == nil {
		t.Fatal("R should load the picker directory")
	}
	if model.taskProfileName != "demo" {
		t.Fatalf("task profile = %q, want demo", model.taskProfileName)
	}

	model.filePickerPhase = phaseReady
	model.filePickerItems = []files.DirEntry{{Name: "ipm.zip", Path: "/tmp/ipm.zip"}}
	model.filePickerIndex = 0
	if cmd := model.chooseFilePickerEntry(); cmd != nil {
		t.Fatal("choosing a dump should not queue a task")
	}
	if model.modal != modalRestoreDatabase {
		t.Fatalf("modal = %v, want restore form", model.modal)
	}
	if model.restoreDestination != "ipm" {
		t.Fatalf("destination = %q, want ipm", model.restoreDestination)
	}
}

func TestRestoreNeedsSelectedProfile(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.pane, model.tab = paneDetails, tabDatabases
	model.profiles = nil
	model.databasePhase = phaseEmpty

	if cmd := model.startRestorePicker(); cmd != nil {
		t.Fatal("restore without a profile should not open the picker")
	}
	if model.modal != modalNone {
		t.Fatalf("modal = %v, want none", model.modal)
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
	model.modal = modalConfirm

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
	view := ansi.Strip(model.View())
	if !strings.Contains(view, "No profiles yet · n to create one") {
		t.Fatalf("empty state = %q", view)
	}
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "shift+up":
		return tea.KeyMsg{Type: tea.KeyShiftUp}
	case "shift+down":
		return tea.KeyMsg{Type: tea.KeyShiftDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func navModel() *Model {
	model := NewModel(context.Background(), nil)
	model.loading = false
	model.profiles = manyProfiles(3)
	return model
}

func TestEnterDetailsAndLeave(t *testing.T) {
	for _, enter := range []string{"enter", "tab", "right", "l"} {
		model := navModel()
		model.updateKey(keyMsg(enter))
		if model.pane != paneDetails {
			t.Fatalf("%s: pane = %v, want details", enter, model.pane)
		}
		for _, leave := range []string{"esc", "tab", "left", "h"} {
			model.pane = paneDetails
			model.updateKey(keyMsg(leave))
			if model.pane != paneProfiles {
				t.Fatalf("%s: pane = %v, want profiles", leave, model.pane)
			}
		}
	}
}

func TestNumberKeysOpenTabAndFocusDetails(t *testing.T) {
	model := navModel()
	model.updateKey(keyMsg("3"))
	if model.pane != paneDetails || model.tab != tabAddons {
		t.Fatalf("pane/tab = %v/%v, want details/add-ons", model.pane, model.tab)
	}
	model.updateKey(keyMsg("esc"))
	model.updateKey(keyMsg("enter"))
	if model.tab != tabAddons {
		t.Fatalf("tab = %v, want the open tab to persist", model.tab)
	}
}

func TestBracketsStepTabs(t *testing.T) {
	model := navModel()
	model.updateKey(keyMsg("2"))
	model.updateKey(keyMsg("]"))
	if model.tab != tabAddons {
		t.Fatalf("tab = %v, want add-ons", model.tab)
	}
	model.updateKey(keyMsg("["))
	model.updateKey(keyMsg("["))
	if model.tab != tabLogs {
		t.Fatalf("tab = %v, want logs", model.tab)
	}
	model.updateKey(keyMsg("["))
	if model.tab != tabConfig {
		t.Fatalf("tab = %v, want wrap to config", model.tab)
	}
}

func TestProfileSwitchKeepsTabAndFocus(t *testing.T) {
	model := navModel()
	model.updateKey(keyMsg("2"))
	for _, key := range []string{"shift+down", "J"} {
		model.updateKey(keyMsg(key))
	}
	if model.profileIndex != 2 || model.pane != paneDetails || model.tab != tabDatabases {
		t.Fatalf("index/pane/tab = %d/%v/%v", model.profileIndex, model.pane, model.tab)
	}
	model.updateKey(keyMsg("shift+up"))
	model.updateKey(keyMsg("K"))
	if model.profileIndex != 0 {
		t.Fatalf("index = %d, want 0", model.profileIndex)
	}
}

func TestProfilePaneArrowsChangeProfile(t *testing.T) {
	model := navModel()
	model.updateKey(keyMsg("j"))
	model.updateKey(keyMsg("down"))
	if model.profileIndex != 2 || model.pane != paneProfiles {
		t.Fatalf("index/pane = %d/%v", model.profileIndex, model.pane)
	}
}

func TestActionsFollowTheFocusedPane(t *testing.T) {
	model := navModel()
	model.profiles[0].State = "running"
	model.databases = []odoo.Database{{Logical: "a", Physical: "p__a"}}
	model.databasePhase = phaseReady

	model.updateKey(keyMsg("2"))
	model.updateKey(keyMsg("d"))
	if model.modal != modalConfirm || !strings.Contains(model.confirmTitle, "database") {
		t.Fatalf("d on the Databases tab should confirm a drop, got %q", model.confirmTitle)
	}
	model.modal = modalNone

	model.updateKey(keyMsg("esc"))
	model.updateKey(keyMsg("d"))
	if model.modal != modalConfirm || !strings.Contains(model.confirmTitle, "Remove profile") {
		t.Fatalf("d on the profile pane should confirm a profile removal, got %q", model.confirmTitle)
	}
}

func TestDetailsCannotBeFocusedWithoutProfiles(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.updateKey(keyMsg("enter"))
	model.updateKey(keyMsg("3"))
	if model.pane != paneProfiles {
		t.Fatalf("pane = %v, want profiles", model.pane)
	}
}

func TestOpenTabControlsLogStream(t *testing.T) {
	model := navModel()
	model.profiles[0].State = "running"
	model.updateKey(keyMsg("2"))
	if model.logCancel != nil {
		t.Fatal("log stream should stop when the Logs tab is left")
	}
	model.updateKey(keyMsg("1"))
	if model.logCancel == nil {
		t.Fatal("log stream should start when the Logs tab opens")
	}
	model.stopLogStream()
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

func TestWindowStartKeepsIndexVisible(t *testing.T) {
	if got := windowStart(5, 2, 10); got != 0 {
		t.Fatalf("small list start = %d, want 0", got)
	}
	if got := windowStart(100, 0, 10); got != 0 {
		t.Fatalf("top start = %d, want 0", got)
	}
	if got := windowStart(100, 50, 10); got != 45 {
		t.Fatalf("middle start = %d, want 45", got)
	}
	if got := windowStart(100, 99, 10); got != 90 {
		t.Fatalf("bottom start = %d, want 90", got)
	}
}

func TestSelectModalWindowsLongLists(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width = 80
	model.height = 20
	model.selectTitle = "many"
	model.selectPhase = phaseReady
	for index := 0; index < 60; index++ {
		model.selectItems = append(model.selectItems, selectItem{Label: "item", Value: "item"})
	}
	model.selectIndex = 30

	rendered := ansi.Strip(strings.Join(model.selectModalLines(), "\n"))
	if !strings.Contains(rendered, "more") {
		t.Fatalf("long list should show more indicators:\n%s", rendered)
	}
}

func dbModel(count int) *Model {
	model := NewModel(context.Background(), nil)
	model.width, model.height = 100, 24
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "demo", State: "running", Version: "18", URL: "http://demo.lidoo.localhost"}}
	model.databasePhase = phaseReady
	model.databaseInfoPhase = phaseReady
	model.databaseInfos = make(map[string]odoo.DatabaseInfo)
	for index := 0; index < count; index++ {
		name := fmt.Sprintf("db%03d", index)
		model.databases = append(model.databases, odoo.Database{Logical: name, Physical: "demo__" + name})
		model.databaseInfos["demo__"+name] = odoo.DatabaseInfo{Size: "60 MB", Available: index%2 == 0}
	}
	model.pane, model.tab = paneDetails, tabDatabases
	return model
}

func TestDatabaseTableColumns(t *testing.T) {
	model := dbModel(2)
	view := ansi.Strip(model.databaseTabView(80, 10))
	for _, want := range []string{"NAME", "SIZE", "STATE", "60 MB", "● ready", "○ unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("database table missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 80 {
			t.Fatalf("line wider than the table: %q", line)
		}
	}
}

func TestDatabaseTableWindowKeepsSelectionVisible(t *testing.T) {
	model := dbModel(100)
	model.databaseIndex = 80
	view := ansi.Strip(model.databaseTabView(80, 8))
	if !strings.Contains(view, "▌ db080") {
		t.Fatalf("selected row is not visible:\n%s", view)
	}
	if got := len(strings.Split(view, "\n")); got > 8 {
		t.Fatalf("table has %d lines, want at most 8", got)
	}
}

func TestClickSelectsDatabaseRow(t *testing.T) {
	model := dbModel(30)
	model.databaseIndex = 25
	lines := strings.Split(ansi.Strip(model.View()), "\n")
	row := -1
	for index, line := range lines {
		if strings.Contains(line, "db023") {
			row = index
		}
	}
	if row < 0 {
		t.Fatalf("db023 not visible:\n%s", strings.Join(lines, "\n"))
	}
	model.pane = paneProfiles
	model.handleLeftClick(60, row)
	if model.databaseIndex != 23 || model.pane != paneDetails {
		t.Fatalf("databaseIndex/pane = %d/%v, want 23/details", model.databaseIndex, model.pane)
	}
}

func TestClickOnTabLabelOpensTab(t *testing.T) {
	model := dbModel(1)
	model.pane, model.tab = paneProfiles, tabLogs
	lines := strings.Split(ansi.Strip(model.View()), "\n")
	for y, line := range lines {
		if index := strings.Index(line, "3 Add-ons"); index >= 0 {
			model.handleLeftClick(ansi.StringWidth(line[:index])+1, y)
			break
		}
	}
	if model.tab != tabAddons || model.pane != paneDetails {
		t.Fatalf("tab/pane = %v/%v, want add-ons/details", model.tab, model.pane)
	}
}

func TestAddonWorktreeShowsParent(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "demo"}}
	model.addonPhase = phaseReady
	model.addons = []addons.AddonStatus{{Name: "feature", Kind: "worktree", Branch: "feat", Entry: addons.Entry{WorktreeOf: "parent-addon"}}}
	view := ansi.Strip(model.addonTabView(80, 10))
	if !strings.Contains(view, "↳ parent-addon") {
		t.Fatalf("worktree should show its parent:\n%s", view)
	}
}

func TestInfoTabListsKeyValues(t *testing.T) {
	model := configModel()
	model.tab = tabInfo
	model.profileDetail.AttachedAddons = []string{"a", "b"}
	model.profileDetail.Image = "lidoo-odoo:18"
	model.profileDetail.OdooVersion = "18.0"
	view := ansi.Strip(model.infoTabView())
	for _, want := range []string{"State", "URL", "Odoo", "18.0 · lidoo-odoo:18", "Add-ons", "a, b", "DB prefix", "Filestore", "LAN port", "Recreation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("info tab missing %q:\n%s", want, view)
		}
	}
}

func TestLogsTabShowsFilterStrip(t *testing.T) {
	model := configModel()
	model.tab = tabLogs
	model.logDatabase = "demo__x"
	view := ansi.Strip(model.logStrip(80))
	for _, want := range []string{"level all", "db demo__x", "follow", "/ search", "L fullscreen"} {
		if !strings.Contains(view, want) {
			t.Fatalf("log strip missing %q: %q", want, view)
		}
	}
}

func TestSlashOpensLogSearch(t *testing.T) {
	model := configModel()
	model.tab = tabLogs
	model.updateKey(keyMsg("/"))
	if !model.logViewerOpen || !model.logViewerTyping {
		t.Fatalf("open/typing = %v/%v, want the viewer open in search mode", model.logViewerOpen, model.logViewerTyping)
	}
}

package tui

import (
	"context"
	"errors"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lidoo/internal/addons"
	"lidoo/internal/app"
	"lidoo/internal/profile"
)

type taskKind uint8

const (
	taskCreate taskKind = iota
	taskRun
	taskStop
	taskRestart
	taskRecreate
	taskRemove
	taskInit
	taskUpdate
	taskDrop
	taskBackup
	taskRestore
	taskAttachAddons
	taskDetachAddons
	taskFetchAddon
	taskPullAddon
	taskAddAddon
	taskWorktreeAddon
	taskRemoveAddon
	taskUpdateConfig
	taskSetAdminPassword
)

type taskRequest struct {
	Kind             taskKind
	ProfileName      string
	DatabaseName     string
	DatabasePhysical string
	Version          string
	Modules          string
	UpdateAll        bool
	Yes              bool
	Destination      string
	Format           string
	Filestore        bool
	IfExists         bool
	Force            bool
	Source           string
	CopyDatabase     bool
	Neutralize       bool
	Jobs             string
	AddonNames       []string
	AddonName        string
	AddonURL         string
	AddonOptions     addons.AddOptions
	AddonSource      string
	AddonBranch      string
	Recreate         bool
	ConfigUpdate     *profile.ConfigUpdate
	Password         string
	Wait             bool
}

// runReadyTimeout bounds the readiness wait that follows a run task.
const runReadyTimeout = 2 * time.Minute

func taskActionLabel(kind taskKind) string {
	switch kind {
	case taskCreate:
		return "creating"
	case taskRun:
		return "starting"
	case taskStop:
		return "stopping"
	case taskRestart:
		return "restarting"
	case taskRecreate:
		return "recreating"
	case taskRemove:
		return "removing"
	case taskInit:
		return "initializing"
	case taskUpdate:
		return "updating"
	case taskDrop:
		return "dropping"
	case taskBackup:
		return "backing up"
	case taskRestore:
		return "restoring"
	case taskAttachAddons:
		return "attaching"
	case taskDetachAddons:
		return "detaching"
	case taskFetchAddon:
		return "fetching"
	case taskPullAddon:
		return "pulling"
	case taskAddAddon:
		return "cloning"
	case taskWorktreeAddon:
		return "creating worktree"
	case taskRemoveAddon:
		return "removing"
	case taskUpdateConfig:
		return "configuring"
	case taskSetAdminPassword:
		return "setting admin password"
	default:
		return "working"
	}
}

func taskLabel(kind taskKind) string {
	switch kind {
	case taskCreate:
		return "create"
	case taskRun:
		return "run"
	case taskStop:
		return "stop"
	case taskRestart:
		return "restart"
	case taskRecreate:
		return "recreate"
	case taskRemove:
		return "remove"
	case taskInit:
		return "init"
	case taskUpdate:
		return "update"
	case taskDrop:
		return "drop"
	case taskBackup:
		return "backup"
	case taskRestore:
		return "restore"
	case taskAttachAddons:
		return "attach add-ons"
	case taskDetachAddons:
		return "detach add-ons"
	case taskFetchAddon:
		return "fetch add-on"
	case taskPullAddon:
		return "pull add-on"
	case taskAddAddon:
		return "clone add-on"
	case taskWorktreeAddon:
		return "create worktree"
	case taskRemoveAddon:
		return "remove add-on"
	case taskUpdateConfig:
		return "configure profile"
	case taskSetAdminPassword:
		return "set admin password"
	default:
		return "profile operation"
	}
}

func StartTaskCmd(id uint64) tea.Cmd {
	return func() tea.Msg {
		return TaskStartedMsg{ID: id}
	}
}

type taskProgressEvent struct {
	ProfileName string
	Text        string
	Result      tea.Msg
}

func ExecuteProfileTaskCmd(ctx context.Context, service *app.Service, request taskRequest, id uint64, progress chan<- taskProgressEvent) tea.Cmd {
	return func() tea.Msg {
		output := &taskOutputWriter{profileName: request.ProfileName, progress: progress}
		profileOptions := app.ProfileOperationOptions{Output: output, ErrorOutput: output}
		databaseOptions := app.DatabaseOperationOptions{Output: output, ErrorOutput: output}
		addonOptions := app.AddonOperationOptions{Output: output, ErrorOutput: output}
		var err error
		if service == nil {
			err = errors.New("workspace service is unavailable")
		} else {
			switch request.Kind {
			case taskCreate:
				err = service.CreateProfile(ctx, request.ProfileName, request.Version)
			case taskRun:
				err = service.RunProfile(ctx, app.RunProfileInput{Name: request.ProfileName, Version: request.Version}, profileOptions)
				if err == nil && request.Wait {
					err = service.WaitProfile(ctx, request.ProfileName, runReadyTimeout, profileOptions)
				}
			case taskStop:
				err = service.StopProfile(ctx, request.ProfileName, profileOptions)
			case taskRestart:
				err = service.RestartProfile(ctx, request.ProfileName, profileOptions)
			case taskRecreate:
				err = service.RecreateProfile(ctx, request.ProfileName, profileOptions)
			case taskRemove:
				err = service.RemoveProfile(ctx, app.RemoveProfileInput{Name: request.ProfileName, Yes: true}, profileOptions)
			case taskInit:
				_, err = service.InitializeDatabase(ctx, request.ProfileName, request.DatabaseName, request.Modules, databaseOptions)
			case taskUpdate:
				_, err = service.UpdateDatabase(ctx, request.ProfileName, request.DatabaseName, request.UpdateAll, databaseOptions)
			case taskDrop:
				_, err = service.DropDatabase(ctx, request.ProfileName, request.DatabaseName, request.Yes, databaseOptions)
			case taskBackup:
				_, err = service.BackupDatabase(ctx, request.ProfileName, request.DatabaseName, request.Destination, request.Format, request.Force, request.IfExists, request.Filestore, databaseOptions)
			case taskRestore:
				jobs, parseErr := strconv.Atoi(request.Jobs)
				if parseErr != nil {
					err = errors.New("restore jobs must be a number")
				} else {
					_, err = service.RestoreDatabase(ctx, request.ProfileName, request.DatabaseName, request.Source, request.CopyDatabase, request.Force, request.Neutralize, jobs, databaseOptions)
				}
			case taskAttachAddons:
				err = service.AttachAddons(ctx, app.AddonMountInput{ProfileName: request.ProfileName, AddonNames: request.AddonNames, Recreate: request.Recreate}, addonOptions)
			case taskDetachAddons:
				err = service.DetachAddons(ctx, app.AddonMountInput{ProfileName: request.ProfileName, AddonNames: request.AddonNames, Recreate: request.Recreate}, addonOptions)
			case taskFetchAddon:
				err = service.FetchAddon(ctx, request.AddonName, addonOptions)
			case taskPullAddon:
				err = service.PullAddon(ctx, request.AddonName, addonOptions)
			case taskAddAddon:
				err = service.AddAddon(ctx, app.AddAddonInput{Name: request.AddonName, URL: request.AddonURL, Options: request.AddonOptions}, addonOptions)
			case taskWorktreeAddon:
				err = service.WorktreeAddon(ctx, app.WorktreeAddonInput{Source: request.AddonSource, Name: request.AddonName, Branch: request.AddonBranch, Yes: request.Yes}, addonOptions)
			case taskRemoveAddon:
				err = service.RemoveAddon(ctx, app.RemoveAddonInput{Name: request.AddonName, Yes: request.Yes, Force: request.Force}, addonOptions)
			case taskUpdateConfig:
				if request.ConfigUpdate == nil {
					err = errors.New("profile configuration update is missing")
				} else {
					err = service.UpdateProfileConfig(ctx, request.ProfileName, *request.ConfigUpdate, profileOptions)
				}
			case taskSetAdminPassword:
				_, err = service.SetAdminPassword(ctx, request.ProfileName, request.DatabaseName, request.Password, databaseOptions)
			default:
				err = errors.New("unknown profile task")
			}
		}
		if errors.Is(err, context.Canceled) {
			progress <- taskProgressEvent{Result: TaskCancelledMsg{ID: id, ProfileName: request.ProfileName}}
		} else if err != nil {
			progress <- taskProgressEvent{Result: TaskFailedMsg{ID: id, ProfileName: request.ProfileName, Err: err}}
		} else {
			progress <- taskProgressEvent{Result: TaskCompletedMsg{ID: id, ProfileName: request.ProfileName}}
		}
		close(progress)
		return nil
	}
}

func WaitTaskProgressCmd(id uint64, progress <-chan taskProgressEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-progress
		if !ok {
			return TaskProgressDoneMsg{ID: id}
		}
		if event.Result != nil {
			return event.Result
		}
		return TaskProgressMsg{ID: id, ProfileName: event.ProfileName, Text: event.Text}
	}
}

type taskOutputWriter struct {
	profileName string
	progress    chan<- taskProgressEvent
}

func (writer *taskOutputWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if writer.progress != nil {
		writer.progress <- taskProgressEvent{ProfileName: writer.profileName, Text: string(data)}
	}
	return len(data), nil
}

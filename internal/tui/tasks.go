package tui

import (
	"context"
	"errors"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"lidoo/internal/app"
)

type taskKind uint8

const (
	taskRun taskKind = iota
	taskStop
	taskRestart
	taskRecreate
	taskRemove
	taskInit
	taskUpdate
	taskDrop
	taskBackup
	taskRestore
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
}

func taskLabel(kind taskKind) string {
	switch kind {
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
		var err error
		if service == nil {
			err = errors.New("workspace service is unavailable")
		} else {
			switch request.Kind {
			case taskRun:
				err = service.RunProfile(ctx, app.RunProfileInput{Name: request.ProfileName, Version: request.Version}, profileOptions)
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

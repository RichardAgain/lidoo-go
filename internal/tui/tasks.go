package tui

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

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

func ExecuteProfileTaskCmd(ctx context.Context, service *app.Service, request taskRequest, id uint64, progress chan<- string) tea.Cmd {
	return func() tea.Msg {
		output := &taskOutputWriter{progress: progress}
		_, _ = output.Write([]byte("running " + taskLabel(request.Kind) + " for " + request.ProfileName + taskDatabaseSuffix(request) + "\n"))
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
		close(progress)
		if errors.Is(err, context.Canceled) {
			return TaskCancelledMsg{ID: id, ProfileName: request.ProfileName, Output: output.String()}
		}
		if err != nil {
			return TaskFailedMsg{ID: id, ProfileName: request.ProfileName, Err: err, Output: output.String()}
		}
		return TaskCompletedMsg{ID: id, ProfileName: request.ProfileName, Output: output.String()}
	}
}

func taskDatabaseSuffix(request taskRequest) string {
	if request.DatabaseName == "" {
		return ""
	}
	return "/" + request.DatabaseName
}

func WaitTaskProgressCmd(id uint64, progress <-chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-progress
		if !ok {
			return TaskProgressDoneMsg{ID: id}
		}
		return TaskProgressMsg{ID: id, Text: text}
	}
}

type taskOutputWriter struct {
	output   bytes.Buffer
	progress chan<- string
	mu       sync.Mutex
}

func (writer *taskOutputWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	count, err := writer.output.Write(data)
	text := strings.TrimSpace(string(data))
	if text != "" && writer.progress != nil {
		select {
		case writer.progress <- text:
		default:
		}
	}
	return count, err
}

func (writer *taskOutputWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return strings.TrimSpace(writer.output.String())
}

package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"

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
)

type taskRequest struct {
	Kind        taskKind
	ProfileName string
	Version     string
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
		_, _ = output.Write([]byte("running " + taskLabel(request.Kind) + " for " + request.ProfileName + "\n"))
		options := app.ProfileOperationOptions{Output: output, ErrorOutput: output}
		var err error
		if service == nil {
			err = errors.New("workspace service is unavailable")
		} else {
			switch request.Kind {
			case taskRun:
				err = service.RunProfile(ctx, app.RunProfileInput{Name: request.ProfileName, Version: request.Version}, options)
			case taskStop:
				err = service.StopProfile(ctx, request.ProfileName, options)
			case taskRestart:
				err = service.RestartProfile(ctx, request.ProfileName, options)
			case taskRecreate:
				err = service.RecreateProfile(ctx, request.ProfileName, options)
			case taskRemove:
				err = service.RemoveProfile(ctx, app.RemoveProfileInput{Name: request.ProfileName, Yes: true}, options)
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
}

func (writer *taskOutputWriter) Write(data []byte) (int, error) {
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
	return strings.TrimSpace(writer.output.String())
}

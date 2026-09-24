package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"lidoo/internal/app"
	"lidoo/internal/odoo"
)

func OpenProfileURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", url)
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			command = exec.Command("xdg-open", url)
		}
		if err := command.Start(); err != nil {
			return ProfileURLOpenFailedMsg{Err: fmt.Errorf("open profile URL %q: %w", url, err)}
		}
		if err := command.Process.Release(); err != nil {
			return ProfileURLOpenFailedMsg{Err: fmt.Errorf("release browser process for %q: %w", url, err)}
		}
		return nil
	}
}

func LoadProfilesCmd(ctx context.Context, service *app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return ProfilesFailedMsg{Err: errors.New("workspace service is unavailable")}
		}
		profiles, err := service.Profiles(ctx)
		if err != nil {
			return ProfilesFailedMsg{Err: err}
		}
		return ProfilesLoadedMsg{Profiles: profiles}
	}
}

func StartProfileWatcherCmd(ctx context.Context, service *app.Service) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return ProfileWatcherDisconnectedMsg{Err: errors.New("workspace service is unavailable")}
		}
		events, eventErrors := service.WatchProfileInvalidations(ctx)
		return ProfileWatcherStartedMsg{Events: events, Errors: eventErrors}
	}
}

func WaitProfileInvalidationCmd(events <-chan app.ProfileInvalidation, eventErrors <-chan error) tea.Cmd {
	return func() tea.Msg {
		for events != nil || eventErrors != nil {
			select {
			case event, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				return ProfileInvalidationMsg{Invalidation: event}
			case err, ok := <-eventErrors:
				if !ok {
					eventErrors = nil
					continue
				}
				if err != nil {
					return ProfileWatcherDisconnectedMsg{Err: err}
				}
			}
		}
		return ProfileWatcherDisconnectedMsg{Err: errors.New("Docker event watcher stopped")}
	}
}

func LoadProfileDetailCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return ProfileDetailFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		detail, err := service.Profile(ctx, profileName)
		if err != nil {
			return ProfileDetailFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return ProfileDetailLoadedMsg{RequestID: requestID, ProfileName: profileName, Detail: detail}
	}
}

func LoadDatabasesCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return DatabasesFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		databases, err := service.Databases(ctx, profileName)
		if err != nil {
			return DatabasesFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return DatabasesLoadedMsg{RequestID: requestID, ProfileName: profileName, Databases: databases}
	}
}

func LoadDatabaseInfoCmd(ctx context.Context, service *app.Service, profileName string, database odoo.Database, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return DatabaseInfoFailedMsg{
				RequestID:        requestID,
				ProfileName:      profileName,
				DatabasePhysical: database.Physical,
				Err:              errors.New("workspace service is unavailable"),
			}
		}
		info, err := service.Database(ctx, profileName, database.Logical)
		if err != nil {
			return DatabaseInfoFailedMsg{
				RequestID:        requestID,
				ProfileName:      profileName,
				DatabasePhysical: database.Physical,
				Err:              err,
			}
		}
		return DatabaseInfoLoadedMsg{
			RequestID:        requestID,
			ProfileName:      profileName,
			DatabasePhysical: database.Physical,
			Info:             info,
		}
	}
}

func LoadAddonsCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return AddonsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		addons, err := service.Addons(ctx, profileName)
		if err != nil {
			return AddonsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return AddonsLoadedMsg{RequestID: requestID, ProfileName: profileName, Addons: addons}
	}
}

func LoadAvailableAddonsCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return AvailableAddonsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		addons, err := service.AvailableAddons(ctx, profileName)
		if err != nil {
			return AvailableAddonsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return AvailableAddonsLoadedMsg{RequestID: requestID, ProfileName: profileName, Addons: addons}
	}
}

func LoadProfileLogsCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return ProfileLogsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: errors.New("workspace service is unavailable")}
		}
		output, err := service.ProfileLogsSinceStart(ctx, profileName)
		if err != nil {
			return ProfileLogsFailedMsg{RequestID: requestID, ProfileName: profileName, Err: err}
		}
		return ProfileLogsLoadedMsg{RequestID: requestID, ProfileName: profileName, Output: output}
	}
}

type profileLogStreamEvent struct {
	Text string
	Err  error
}

func FollowProfileLogsCmd(ctx context.Context, service *app.Service, profileName string, requestID uint64, stream chan<- profileLogStreamEvent) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		if service == nil {
			stream <- profileLogStreamEvent{Err: errors.New("workspace service is unavailable")}
			close(stream)
			return nil
		}
		command, err := service.ProfileLogsCommand(ctx, profileName, true, 0)
		if err == nil {
			writer := profileLogStreamWriter{ctx: ctx, stream: stream}
			command.Stdout = writer
			command.Stderr = writer
			err = command.Run()
		}
		if err != nil && ctx.Err() == nil {
			stream <- profileLogStreamEvent{Err: err}
		}
		close(stream)
		return nil
	}
}

func WaitProfileLogStreamCmd(profileName string, requestID uint64, stream <-chan profileLogStreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-stream
		if !ok {
			return ProfileLogStreamDoneMsg{RequestID: requestID, ProfileName: profileName}
		}
		if event.Err != nil {
			return ProfileLogStreamFailedMsg{RequestID: requestID, ProfileName: profileName, Err: event.Err}
		}
		return ProfileLogStreamMsg{RequestID: requestID, ProfileName: profileName, Text: event.Text}
	}
}

type profileLogStreamWriter struct {
	ctx    context.Context
	stream chan<- profileLogStreamEvent
}

func (writer profileLogStreamWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	select {
	case writer.stream <- profileLogStreamEvent{Text: string(data)}:
	case <-writer.ctx.Done():
	}
	return len(data), nil
}

func PrepareDatabaseShellCmd(ctx context.Context, service *app.Service, profileName, databaseName string) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return InteractiveCommandFailedMsg{Kind: interactiveDatabaseShell, ProfileName: profileName, DatabaseName: databaseName, Err: errors.New("workspace service is unavailable")}
		}
		command, err := service.DatabaseShellCommand(ctx, profileName, databaseName)
		if err != nil {
			return InteractiveCommandFailedMsg{Kind: interactiveDatabaseShell, ProfileName: profileName, DatabaseName: databaseName, Err: err}
		}
		return InteractiveCommandReadyMsg{Kind: interactiveDatabaseShell, ProfileName: profileName, DatabaseName: databaseName, Command: command}
	}
}

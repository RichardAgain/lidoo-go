package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"lidoo/internal/app"
)

func Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	service, err := app.Open()
	if err != nil {
		return err
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	program := tea.NewProgram(NewModel(sessionCtx, service), tea.WithAltScreen(), tea.WithContext(sessionCtx))
	_, err = program.Run()
	return err
}

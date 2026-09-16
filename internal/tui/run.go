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
	program := tea.NewProgram(NewModel(ctx, service), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err = program.Run()
	return err
}

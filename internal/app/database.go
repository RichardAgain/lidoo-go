package app

import (
	"context"
	"io"

	"lidoo/internal/files"
	"lidoo/internal/odoo"
)

// DatabaseOperationOptions keeps output routing at the presentation boundary.
// The TUI can provide a task writer while the CLI uses the process streams.
type DatabaseOperationOptions struct {
	Output      io.Writer
	ErrorOutput io.Writer
}

type DatabaseOperationResult = odoo.OperationResult

func normalizeDatabaseOperationOptions(options []DatabaseOperationOptions) DatabaseOperationOptions {
	var result DatabaseOperationOptions
	if len(options) > 0 {
		result = options[0]
	}
	return result
}

func (s *Service) InitializeDatabase(ctx context.Context, name, database, modules string, options ...DatabaseOperationOptions) (DatabaseOperationResult, error) {
	operationOptions := normalizeDatabaseOperationOptions(options)
	return s.withDatabaseOperation(ctx, func(state files.State) (DatabaseOperationResult, error) {
		return odoo.Init(name, database, modules, state, odoo.OperationOptions{
			Context:     ctx,
			Output:      operationOptions.Output,
			ErrorOutput: operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) UpdateDatabase(ctx context.Context, name, database string, updateAll bool, options ...DatabaseOperationOptions) (DatabaseOperationResult, error) {
	operationOptions := normalizeDatabaseOperationOptions(options)
	return s.withDatabaseOperation(ctx, func(state files.State) (DatabaseOperationResult, error) {
		return odoo.Update(name, database, updateAll, state, odoo.OperationOptions{
			Context:     ctx,
			Output:      operationOptions.Output,
			ErrorOutput: operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) DropDatabase(ctx context.Context, name, database string, yes bool, options ...DatabaseOperationOptions) (DatabaseOperationResult, error) {
	operationOptions := normalizeDatabaseOperationOptions(options)
	return s.withDatabaseOperation(ctx, func(state files.State) (DatabaseOperationResult, error) {
		return odoo.Drop(name, database, yes, state, odoo.OperationOptions{
			Context:     ctx,
			Output:      operationOptions.Output,
			ErrorOutput: operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) BackupDatabase(ctx context.Context, name, database, destination, format string, force, ifExists, filestore bool, options ...DatabaseOperationOptions) (DatabaseOperationResult, error) {
	operationOptions := normalizeDatabaseOperationOptions(options)
	return s.withDatabaseOperation(ctx, func(state files.State) (DatabaseOperationResult, error) {
		return odoo.Backup(name, database, destination, format, force, ifExists, filestore, state, odoo.OperationOptions{
			Context:     ctx,
			Output:      operationOptions.Output,
			ErrorOutput: operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) RestoreDatabase(ctx context.Context, name, database, source string, copyDatabase, force, neutralize bool, jobs int, options ...DatabaseOperationOptions) (DatabaseOperationResult, error) {
	operationOptions := normalizeDatabaseOperationOptions(options)
	return s.withDatabaseOperation(ctx, func(state files.State) (DatabaseOperationResult, error) {
		return odoo.Restore(name, database, source, copyDatabase, force, neutralize, jobs, state, odoo.OperationOptions{
			Context:     ctx,
			Output:      operationOptions.Output,
			ErrorOutput: operationOptions.ErrorOutput,
		})
	})
}

func (s *Service) withDatabaseOperation(ctx context.Context, operation func(files.State) (DatabaseOperationResult, error)) (DatabaseOperationResult, error) {
	var result DatabaseOperationResult
	err := s.withWorkspaceOperation(ctx, false, func(state files.State) error {
		var operationErr error
		result, operationErr = operation(state)
		return operationErr
	})
	return result, err
}

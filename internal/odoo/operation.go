package odoo

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"

	"lidoo/internal/docker"
)

// OperationOptions carries the context and output streams for a database
// operation. Database workflows never write to process-global terminal
// streams; callers choose where progress and diagnostics should go.
type OperationOptions struct {
	Context     context.Context
	Output      io.Writer
	ErrorOutput io.Writer
}

// OperationResult contains the authoritative target and output produced by a
// database operation. Output is also streamed to the writers in
// OperationOptions while the operation runs.
type OperationResult struct {
	ProfileName      string
	LogicalDatabase  string
	PhysicalDatabase string
	Destination      string
	Skipped          bool
	Output           string
	ErrorOutput      string
}

type operationOutput struct {
	destination io.Writer
	buffer      bytes.Buffer
	mu          sync.Mutex
}

func newOperationStreams(options []OperationOptions) (*operationOutput, *operationOutput, OperationOptions) {
	normalized := normalizeOperationOptions(options)
	return &operationOutput{destination: normalized.Output}, &operationOutput{destination: normalized.ErrorOutput}, normalized
}

func normalizeOperationOptions(options []OperationOptions) OperationOptions {
	var normalized OperationOptions
	if len(options) > 0 {
		normalized = options[0]
	}
	if normalized.Context == nil {
		normalized.Context = context.Background()
	}
	if normalized.Output == nil {
		normalized.Output = os.Stdout
	}
	if normalized.ErrorOutput == nil {
		normalized.ErrorOutput = os.Stderr
	}
	return normalized
}

func (output *operationOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()

	if _, err := output.buffer.Write(data); err != nil {
		return 0, err
	}
	if output.destination == nil {
		return len(data), nil
	}
	written, err := output.destination.Write(data)
	if err != nil {
		return written, err
	}
	if written != len(data) {
		return written, io.ErrShortWrite
	}
	return written, nil
}

func (output *operationOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

func operationResult(profileName string, stdout, stderr *operationOutput) OperationResult {
	return OperationResult{
		ProfileName: profileName,
		Output:      stdout.String(),
		ErrorOutput: stderr.String(),
	}
}

func (output *operationOutput) commandOptions(stderr *operationOutput, context context.Context) docker.CommandOptions {
	return docker.CommandOptions{
		Context: context,
		Stdout:  output,
		Stderr:  stderr,
	}
}

// Package eventlooptest provides shared support for live eventloop tests.
package eventlooptest

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout reports that a lifecycle test operation exceeded its cleanup
// bound.
var ErrTimeout = errors.New("eventloop test lifecycle operation timed out")

// TerminalLoop is the lifecycle surface required by Terminate.
type TerminalLoop interface {
	Shutdown(context.Context) error
	Close() error
}

// TerminalResult records each independently bounded terminal operation.
type TerminalResult struct {
	ShutdownErr error
	CloseErr    error
	RunErr      error
}

// Terminate requests graceful shutdown, falls back to Close, and joins Run.
// Every possibly blocking call has an independent bound so a failed test
// cannot hang the package process indefinitely.
func Terminate(loop TerminalLoop, runDone <-chan error, timeout time.Duration) TerminalResult {
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), timeout)
	shutdownErr := await(func() error { return loop.Shutdown(shutdownContext) }, timeout)
	shutdownCancel()

	var closeErr error
	if shutdownErr != nil {
		closeErr = await(loop.Close, timeout)
	}
	runErr := await(func() error { return <-runDone }, timeout)
	return TerminalResult{ShutdownErr: shutdownErr, CloseErr: closeErr, RunErr: runErr}
}

func await(operation func() error, timeout time.Duration) error {
	result := make(chan error, 1)
	go func() { result <- operation() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return ErrTimeout
	}
}

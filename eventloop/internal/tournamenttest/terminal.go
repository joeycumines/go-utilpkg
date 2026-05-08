// Package tournamenttest provides shared lifecycle machinery for the retained
// longitudinal tournament implementations and harnesses.
package tournamenttest

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout reports that a tournament lifecycle operation did not complete
// within its explicit cleanup bound.
var ErrTimeout = errors.New("tournament lifecycle operation timed out")

// TerminalLoop is the terminal lifecycle surface shared by tournament loops.
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

// Terminate requests graceful shutdown, falls back to Close when shutdown does
// not succeed, and joins Run. Every possibly blocking call has its own bound so
// a broken historical implementation cannot hang the tournament process.
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

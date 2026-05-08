package gojaeventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errBenchmarkLifecycleTimeout = errors.New("benchmark lifecycle operation timed out")

type benchmarkTerminalLoop interface {
	Shutdown(context.Context) error
	Close() error
}

type benchmarkTerminalResult struct {
	shutdownErr error
	closeErr    error
	runErr      error
}

func terminateBenchmarkLoop(loop benchmarkTerminalLoop, runDone <-chan error, timeout time.Duration) benchmarkTerminalResult {
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), timeout)
	shutdownErr := awaitBenchmarkLifecycle(func() error { return loop.Shutdown(shutdownContext) }, timeout)
	shutdownCancel()
	var closeErr error
	if shutdownErr != nil {
		closeErr = awaitBenchmarkLifecycle(loop.Close, timeout)
	}
	runErr := awaitBenchmarkLifecycle(func() error { return <-runDone }, timeout)
	return benchmarkTerminalResult{shutdownErr: shutdownErr, closeErr: closeErr, runErr: runErr}
}

func awaitBenchmarkLifecycle(operation func() error, timeout time.Duration) error {
	result := make(chan error, 1)
	go func() { result <- operation() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return errBenchmarkLifecycleTimeout
	}
}

func TestTerminateBenchmarkLoopBoundsEveryOperation(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	loop := benchmarkTerminalLoopFixture{
		shutdown: func(context.Context) error {
			<-release
			return nil
		},
		close: func() error {
			<-release
			return nil
		},
	}
	result := terminateBenchmarkLoop(loop, make(chan error), time.Millisecond)
	if !errors.Is(result.shutdownErr, errBenchmarkLifecycleTimeout) {
		t.Fatalf("Shutdown error = %v, want timeout", result.shutdownErr)
	}
	if !errors.Is(result.closeErr, errBenchmarkLifecycleTimeout) {
		t.Fatalf("Close error = %v, want timeout", result.closeErr)
	}
	if !errors.Is(result.runErr, errBenchmarkLifecycleTimeout) {
		t.Fatalf("Run error = %v, want timeout", result.runErr)
	}
}

type benchmarkTerminalLoopFixture struct {
	shutdown func(context.Context) error
	close    func() error
}

func (f benchmarkTerminalLoopFixture) Shutdown(ctx context.Context) error {
	return f.shutdown(ctx)
}

func (f benchmarkTerminalLoopFixture) Close() error {
	return f.close()
}

package tournamenttest

import (
	"context"
	"errors"
	"testing"
	"time"
)

type terminalLoopFixture struct {
	shutdown func(context.Context) error
	close    func() error
}

func (f terminalLoopFixture) Shutdown(ctx context.Context) error {
	return f.shutdown(ctx)
}

func (f terminalLoopFixture) Close() error {
	return f.close()
}

func TestTerminateCleanShutdown(t *testing.T) {
	runDone := make(chan error, 1)
	runDone <- nil
	closeCalled := false
	result := Terminate(terminalLoopFixture{
		shutdown: func(context.Context) error { return nil },
		close: func() error {
			closeCalled = true
			return nil
		},
	}, runDone, time.Second)
	if result != (TerminalResult{}) {
		t.Fatalf("Terminate result = %+v, want no errors", result)
	}
	if closeCalled {
		t.Fatal("Close called after successful Shutdown")
	}
}

func TestTerminateBoundsStuckShutdownAndJoinsRun(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	runDone := make(chan error, 1)
	runDone <- nil
	result := Terminate(terminalLoopFixture{
		shutdown: func(context.Context) error {
			<-release
			return nil
		},
		close: func() error { return nil },
	}, runDone, time.Millisecond)
	if !errors.Is(result.ShutdownErr, ErrTimeout) {
		t.Fatalf("Shutdown error = %v, want ErrTimeout", result.ShutdownErr)
	}
	if result.CloseErr != nil || result.RunErr != nil {
		t.Fatalf("fallback result = %+v, want successful Close and Run join", result)
	}
}

func TestTerminateBoundsFallbackCloseAndRunJoin(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	shutdownErr := errors.New("shutdown failed")
	result := Terminate(terminalLoopFixture{
		shutdown: func(context.Context) error { return shutdownErr },
		close: func() error {
			<-release
			return nil
		},
	}, make(chan error), time.Millisecond)
	if !errors.Is(result.ShutdownErr, shutdownErr) {
		t.Fatalf("Shutdown error = %v, want %v", result.ShutdownErr, shutdownErr)
	}
	if !errors.Is(result.CloseErr, ErrTimeout) {
		t.Fatalf("Close error = %v, want ErrTimeout", result.CloseErr)
	}
	if !errors.Is(result.RunErr, ErrTimeout) {
		t.Fatalf("Run error = %v, want ErrTimeout", result.RunErr)
	}
}

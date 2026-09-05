package eventloop

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func assertPromiseRejected(t *testing.T, promise Future, want error) {
	t.Helper()
	if state := promise.State(); state != Rejected {
		t.Fatalf("promise state = %v, want Rejected", state)
	}
	if !errorResultIs(promise.Result(), want) {
		t.Fatalf("promise result = %v, want %v", promise.Result(), want)
	}
}

func errorResultIs(result any, want error) bool {
	err, ok := result.(error)
	return ok && errors.Is(err, want)
}

func assertCloseSignals(t *testing.T, loop *Loop) {
	t.Helper()
	if loop.Alive() {
		t.Error("Alive returned true after Close")
	}
	if loop.HasMacrotaskWork() {
		t.Error("HasMacrotaskWork returned true after Close")
	}
	select {
	case <-loop.loopDone:
	default:
		t.Fatal("loopDone remained open after Close returned")
	}
	select {
	case <-loop.terminalDone:
	default:
		t.Fatal("terminalDone remained open after Close returned")
	}
}

func waitPromisifyWorkersT(t *testing.T, loop *Loop) {
	t.Helper()
	workersDone := make(chan struct{})
	go func() {
		loop.promisifyWg.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "Promisify worker completion")
	if got := loop.promisifyCount.Load(); got != 0 {
		t.Fatalf("promisifyCount = %d after workers completed, want 0", got)
	}
}

func registerLoopCleanupT(t *testing.T, loop *Loop) {
	t.Helper()
	t.Cleanup(func() {
		state := loop.State()
		if state != StateTerminating && state != StateTerminated {
			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			select {
			case err := <-closeDone:
				if err != nil && !errors.Is(err, ErrLoopTerminated) {
					t.Errorf("cleanup Close: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("cleanup Close did not return")
			}
		}
		select {
		case <-loop.terminalDone:
		case <-time.After(5 * time.Second):
			t.Error("cleanup timed out waiting for terminal completion")
		}
		waitPromisifyWorkersT(t, loop)
	})
}

func registerActiveLoopCleanupT(t *testing.T, loop *Loop, runDone <-chan error) {
	t.Helper()
	t.Cleanup(func() {
		state := loop.State()
		if state != StateTerminating && state != StateTerminated {
			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			select {
			case err := <-closeDone:
				if err != nil && !errors.Is(err, ErrLoopTerminated) {
					t.Errorf("cleanup Close: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("cleanup Close did not return")
			}
		}
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("cleanup Run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("cleanup Run did not return")
		}
		select {
		case <-loop.terminalDone:
		case <-time.After(5 * time.Second):
			t.Error("cleanup timed out waiting for terminal completion")
		}
		waitPromisifyWorkersT(t, loop)
	})
}

func closeFDResourcesT(t testing.TB, loop *Loop) {
	t.Helper()
	loop.closeFDs()
	if err := loop.fdResourceCloseError(); err != nil {
		t.Errorf("descriptor cleanup: %v", err)
	}
}

func registerFDResourceCleanupT(t testing.TB, loop *Loop) {
	t.Helper()
	t.Cleanup(func() { closeFDResourcesT(t, loop) })
}

func releaseSignalT(t *testing.T, signal chan struct{}) func() {
	t.Helper()
	var once sync.Once
	release := func() { once.Do(func() { close(signal) }) }
	t.Cleanup(release)
	return release
}

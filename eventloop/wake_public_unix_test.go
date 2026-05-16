//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWakeSubmitsOnlySelectedNativeWait(t *testing.T) {
	tests := []struct {
		name       string
		mode       FastPathMode
		userFDs    int32
		wantWrites int
		wantState  uint32
	}{
		{name: "auto fast wait", mode: FastPathAuto, wantState: wakeSignalIdle},
		{name: "forced fast wait", mode: FastPathForced, wantState: wakeSignalIdle},
		{name: "disabled native wait", mode: FastPathDisabled, wantWrites: 1, wantState: wakeSignalPending},
		{name: "auto user descriptor", mode: FastPathAuto, userFDs: 1, wantWrites: 1, wantState: wakeSignalPending},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			if err := loop.ensurePoller(); err != nil {
				t.Fatal(err)
			}
			registerFDResourceCleanupT(t, loop)
			loop.fastPathMode.Store(int32(test.mode))
			loop.userIOFDCount.Store(test.userFDs)
			loop.state.Store(StateSleeping)
			writes := 0
			loop.testHooks = &loopTestHooks{WriteWakeFD: func(_ int, value []byte) (int, error) {
				writes++
				return len(value), nil
			}}

			if err := loop.Wake(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-loop.fastWakeupCh:
			default:
				t.Fatal("Wake did not signal the fast waiter")
			}
			if writes != test.wantWrites {
				t.Fatalf("physical writes = %d, want %d", writes, test.wantWrites)
			}
			if state := loop.wakeUpSignalPending.Load(); state != test.wantState {
				t.Fatalf("pending state = %d, want %d", state, test.wantState)
			}
		})
	}
}

func TestWakePreservesPhysicalSubmissionError(t *testing.T) {
	for _, test := range []struct {
		name  string
		state LoopState
	}{
		{name: "running", state: StateRunning},
		{name: "sleeping", state: StateSleeping},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			if err := loop.ensurePoller(); err != nil {
				t.Fatal(err)
			}
			registerFDResourceCleanupT(t, loop)
			loop.state.Store(test.state)
			loop.userIOFDCount.Store(1)

			sentinel := errors.New("wake write failed")
			writes := 0
			loop.testHooks = &loopTestHooks{
				WriteWakeFD: func(_ int, value []byte) (int, error) {
					writes++
					if writes == 1 {
						return 0, sentinel
					}
					return len(value), nil
				},
			}
			if err := loop.Wake(); !errors.Is(err, sentinel) {
				t.Fatalf("Wake error = %v, want %v", err, sentinel)
			}
			if writes != 1 {
				t.Fatalf("physical writes after failed Wake = %d, want 1", writes)
			}
			if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
				t.Fatalf("pending state after failed Wake = %d, want idle", got)
			}

			if err := loop.Wake(); err != nil {
				t.Fatalf("Wake retry = %v", err)
			}
			if writes != 2 {
				t.Fatalf("physical writes after Wake retry = %d, want 2", writes)
			}
			if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
				t.Fatalf("pending state after Wake retry = %d, want pending", got)
			}
		})
	}
}

func TestWakeBeforeSleepingCommitSubmitsSelectedNativeSignal(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.drainWakeUpPipe()
	select {
	case <-loop.fastWakeupCh:
	default:
	}
	loop.state.Store(StateRunning)

	preSleepReached := make(chan struct{})
	releaseSleep := make(chan struct{})
	releaseSleepFn := releaseSignalT(t, releaseSleep)
	pollDone := make(chan struct{})
	var preSleepOnce sync.Once
	var writes atomic.Int32
	var pollCalls atomic.Int32
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			preSleepOnce.Do(func() {
				close(preSleepReached)
				<-releaseSleep
			})
		},
		WriteWakeFD: func(_ int, payload []byte) (int, error) {
			writes.Add(1)
			return len(payload), nil
		},
		PollIO: func(int) (int, error) {
			pollCalls.Add(1)
			return 0, nil
		},
	}
	go func() {
		loop.poll()
		close(pollDone)
	}()
	t.Cleanup(func() {
		releaseSleepFn()
		waitContractSignal(t, pollDone, "poll cleanup after Wake race")
	})

	waitContractSignal(t, preSleepReached, "pre-sleep Wake boundary")
	if err := loop.Wake(); err != nil {
		t.Fatalf("Wake before sleeping commit: %v", err)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("native wake writes before sleeping commit = %d, want 1", got)
	}
	releaseSleepFn()
	waitContractSignal(t, pollDone, "poll completion after Wake")
	if got := pollCalls.Load(); got != 0 {
		t.Fatalf("PollIO calls after committed Wake = %d, want 0", got)
	}
}

func TestWakeCrossingTerminalCommitRemainsNoOp(t *testing.T) {
	tests := []struct {
		name  string
		state LoopState
	}{
		{name: "running", state: StateRunning},
		{name: "sleeping", state: StateSleeping},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New(WithFastPathMode(FastPathDisabled))
			if err != nil {
				t.Fatal(err)
			}
			if err := loop.ensurePoller(); err != nil {
				t.Fatal(err)
			}
			registerFDResourceCleanupT(t, loop)
			loop.state.Store(test.state)

			wakeLockReached := make(chan struct{})
			releaseWake := make(chan struct{})
			releaseWakeFn := releaseSignalT(t, releaseWake)
			loop.testHooks = &loopTestHooks{
				BeforePendingWakeLock: func() {
					close(wakeLockReached)
					<-releaseWake
				},
			}
			wakeDone := make(chan error, 1)
			go func() { wakeDone <- loop.Wake() }()

			waitContractSignal(t, wakeLockReached, "Wake native-lock boundary")
			loop.state.Store(StateTerminated)
			releaseWakeFn()
			select {
			case err := <-wakeDone:
				if err != nil {
					t.Fatalf("Wake crossing terminal commit = %v, want nil", err)
				}
			case <-t.Context().Done():
				t.Fatal("Wake did not return after terminal commit")
			}
			if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
				t.Fatalf("pending state after terminal no-op = %d, want idle", got)
			}
		})
	}
}

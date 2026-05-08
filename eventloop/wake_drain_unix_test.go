//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
	"golang.org/x/sys/unix"
)

func TestWakeDrainSteadyStateAllocations(t *testing.T) {
	loop := New()
	if err := loop.ensurePoller(); err != nil {
		t.Fatalf("ensurePoller: %v", err)
	}
	registerFDResourceCleanupT(t, loop)

	writeBuffer := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	if written, err := unix.Write(loop.wakePipeWrite, writeBuffer); err != nil || written != len(writeBuffer) {
		t.Fatalf("warm wake write = (%d, %v), want (%d, nil)", written, err, len(writeBuffer))
	}
	loop.drainWakeUpPipe()

	var writeErr error
	allocations := testing.AllocsPerRun(100, func() {
		if writeErr != nil {
			return
		}
		written, err := unix.Write(loop.wakePipeWrite, writeBuffer)
		if err != nil {
			writeErr = err
			return
		}
		if written != len(writeBuffer) {
			writeErr = io.ErrShortWrite
			return
		}
		loop.drainWakeUpPipe()
	})
	if writeErr != nil {
		t.Fatalf("wake write during allocation run: %v", writeErr)
	}
	if allocations != 0 {
		t.Fatalf("wake write and drain allocations = %f, want 0", allocations)
	}
}

func TestWakeDrainRetriesInterruptedReadBeforeReset(t *testing.T) {
	loop := New()
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	calls := 0
	loop.testHooks = &loopTestHooks{
		ReadWakeFD: func(_ int, _ []byte) (int, error) {
			calls++
			if calls == 1 {
				return 0, unix.EINTR
			}
			return 0, unix.EAGAIN
		},
	}
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	loop.drainWakeUpPipe()
	if calls != 2 {
		t.Fatalf("wake read calls = %d, want 2", calls)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
		t.Fatalf("pending flag after complete drain = %d, want idle", got)
	}
}

func TestWakeDrainReadResultPolicy(t *testing.T) {
	sentinel := errors.New("unexpected wake read failure")
	tests := []struct {
		name    string
		results []struct {
			n   int
			err error
		}
		wantCalls int
		wantLogs  int32
	}{
		{
			name: "wrapped-interrupted-then-complete",
			results: []struct {
				n   int
				err error
			}{{err: fmt.Errorf("wrapped: %w", unix.EINTR)}, {err: fmt.Errorf("wrapped: %w", unix.EAGAIN)}},
			wantCalls: 2,
		},
		{
			name: "zero-byte-success",
			results: []struct {
				n   int
				err error
			}{{}, {err: unix.EAGAIN}},
			wantCalls: 1,
			wantLogs:  1,
		},
		{
			name: "eof",
			results: []struct {
				n   int
				err error
			}{{err: io.EOF}},
			wantCalls: 1,
			wantLogs:  1,
		},
		{
			name: "unexpected",
			results: []struct {
				n   int
				err error
			}{{err: sentinel}},
			wantCalls: 1,
			wantLogs:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs atomic.Int32
			typedLogger := logiface.New[*testEvent](
				logiface.WithEventFactory[*testEvent](&testEventFactory{}),
				logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
					logs.Add(1)
					return nil
				})),
			)
			loop := New(WithLogger(typedLogger.Logger()))
			if err := loop.ensurePoller(); err != nil {
				t.Fatal(err)
			}
			registerFDResourceCleanupT(t, loop)

			calls := 0
			loop.testHooks = &loopTestHooks{
				ReadWakeFD: func(_ int, _ []byte) (int, error) {
					if calls >= len(test.results) {
						return 0, unix.EAGAIN
					}
					result := test.results[calls]
					calls++
					return result.n, result.err
				},
			}
			loop.wakeUpSignalPending.Store(wakeSignalPending)

			loop.drainWakeUpPipe()
			if calls != test.wantCalls {
				t.Fatalf("wake read calls = %d, want %d", calls, test.wantCalls)
			}
			if got := logs.Load(); got != test.wantLogs {
				t.Fatalf("wake read error logs = %d, want %d", got, test.wantLogs)
			}
			if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
				t.Fatalf("pending state after drain = %d, want idle", got)
			}
		})
	}
}

func TestWakeDrainAfterCloseDoesNotUseStaleDescriptors(t *testing.T) {
	loop := New()
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	closeFDResourcesT(t, loop)

	if loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("wake descriptors after close = (%d, %d), want (-1, -1)", loop.wakePipe, loop.wakePipeWrite)
	}
	var reads atomic.Int32
	loop.testHooks = &loopTestHooks{
		ReadWakeFD: func(_ int, _ []byte) (int, error) {
			reads.Add(1)
			return 0, unix.EAGAIN
		},
	}
	loop.wakeUpSignalPending.Store(wakeSignalPending)
	loop.drainWakeUpPipe()
	if got := reads.Load(); got != 0 {
		t.Fatalf("wake reads after resource close = %d, want 0", got)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
		t.Fatalf("pending state after post-close drain = %d, want idle", got)
	}
}

func TestWakeDrainHoldsResourceUntilReadReturns(t *testing.T) {
	loop := New()
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	loop.state.Store(StateAwake)

	readEntered := make(chan struct{})
	closeReachedResource := make(chan struct{})
	releaseRead := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseRead) })
		closeFDResourcesT(t, loop)
	})
	loop.testHooks = &loopTestHooks{
		BeforeWakeResourceClose: func() { close(closeReachedResource) },
		ReadWakeFD: func(_ int, _ []byte) (int, error) {
			close(readEntered)
			<-releaseRead
			return 0, unix.EAGAIN
		},
	}
	loop.wakeUpSignalPending.Store(wakeSignalPending)
	drainDone := make(chan struct{})
	go func() {
		loop.drainWakeUpPipe()
		close(drainDone)
	}()
	select {
	case <-readEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("wake drain did not enter descriptor read")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case <-closeReachedResource:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not reach wake-resource teardown")
	}
	if loop.wakeMu.TryLock() {
		loop.wakeMu.Unlock()
		releaseOnce.Do(func() { close(releaseRead) })
		<-drainDone
		t.Fatal("descriptor read did not retain wake-resource ownership")
	}

	releaseOnce.Do(func() { close(releaseRead) })
	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		t.Fatal("wake drain did not finish")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish after wake drain released the resource")
	}
	if loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("wake descriptors after drain/close = (%d, %d), want (-1, -1)", loop.wakePipe, loop.wakePipeWrite)
	}
}

func TestWakeDrainLogsOutsideResourceLock(t *testing.T) {
	sentinel := errors.New("wake read failed")
	closeResult := make(chan error, 1)
	var loop *Loop
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			closeResult <- loop.Close()
			return nil
		})),
	)
	loop = New(WithLogger(typedLogger.Logger()))
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.testHooks = &loopTestHooks{
		ReadWakeFD: func(_ int, _ []byte) (int, error) { return 0, sentinel },
	}

	drainDone := make(chan struct{})
	go func() {
		loop.drainWakeUpPipe()
		close(drainDone)
	}()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("reentrant Close from wake diagnostic: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wake diagnostic retained resource lock during logger call")
	}
	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		t.Fatal("wake drain did not return after diagnostic")
	}
}

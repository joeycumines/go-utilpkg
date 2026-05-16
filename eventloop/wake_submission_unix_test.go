//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestWakeSubmissionHoldsDescriptorUntilWriteReturns(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}

	writeEntered := make(chan struct{})
	closeReachedResource := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseWrite) })
		closeFDResourcesT(t, loop)
	})
	loop.testHooks = &loopTestHooks{
		BeforeWakeResourceClose: func() { close(closeReachedResource) },
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			close(writeEntered)
			<-releaseWrite
			return len(p), nil
		},
	}

	wakeDone := make(chan error, 1)
	go func() { wakeDone <- loop.submitWakeupPhysical() }()
	select {
	case <-writeEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("physical wake did not reach the descriptor write")
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
		releaseOnce.Do(func() { close(releaseWrite) })
		if err := waitContractValue(t, wakeDone, "physical wake after unexpected resource-lock acquisition"); err != nil {
			t.Fatalf("physical wake after unexpected resource-lock acquisition: %v", err)
		}
		t.Fatal("descriptor write did not retain wake-resource ownership")
	}

	releaseOnce.Do(func() { close(releaseWrite) })
	select {
	case err := <-wakeDone:
		if err != nil {
			t.Fatalf("physical wake failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("physical wake did not finish")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish after the descriptor write")
	}
}

func TestWakeSubmissionWriteResultPolicy(t *testing.T) {
	type writeResult struct {
		n   int
		err error
	}
	sentinel := errors.New("wake write failed")
	tests := []struct {
		name      string
		results   []writeResult
		wantCalls int
		wantErr   error
	}{
		{name: "interrupted-then-success", results: []writeResult{{err: unix.EINTR}, {n: 8}}, wantCalls: 2},
		{name: "already-pending", results: []writeResult{{err: unix.EAGAIN}}, wantCalls: 1},
		{name: "short-write", results: []writeResult{{n: 7}}, wantCalls: 1, wantErr: io.ErrShortWrite},
		{name: "unexpected-error", results: []writeResult{{err: sentinel}}, wantCalls: 1, wantErr: sentinel},
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

			calls := 0
			loop.testHooks = &loopTestHooks{
				WriteWakeFD: func(_ int, p []byte) (int, error) {
					if calls >= len(test.results) {
						t.Fatalf("unexpected wake write call %d", calls+1)
					}
					result := test.results[calls]
					calls++
					return result.n, result.err
				},
			}

			err = loop.submitWakeupPhysical()
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("submitWakeupPhysical error = %v, want %v", err, test.wantErr)
			}
			if calls != test.wantCalls {
				t.Fatalf("wake write calls = %d, want %d", calls, test.wantCalls)
			}
		})
	}
}

func TestWakeSubmissionDeduplicatesPendingEpoch(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	writes := 0
	loop.testHooks = &loopTestHooks{
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			writes++
			return len(p), nil
		},
	}
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)

	loop.wakeAfterIngress()
	loop.wakeAfterIngress()
	loop.wakeAfterIngress()
	if writes != 1 {
		t.Fatalf("physical writes in one pending epoch = %d, want 1", writes)
	}

	loop.wakeUpSignalPending.Store(wakeSignalIdle)
	loop.wakeAfterIngress()
	if writes != 2 {
		t.Fatalf("physical writes after pending reset = %d, want 2", writes)
	}
}

func TestConcurrentSuccessfulWakeContendersWriteOnce(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)

	var writes atomic.Int32
	loop.testHooks = &loopTestHooks{
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			writes.Add(1)
			return len(p), nil
		},
	}
	const contenders = 64
	start := make(chan struct{})
	errs := make(chan error, contenders)
	var wg sync.WaitGroup
	for range contenders {
		wg.Go(func() {
			<-start
			errs <- loop.submitPendingWakeup()
		})
	}
	close(start)
	joined := make(chan struct{})
	go func() {
		wg.Wait()
		close(joined)
	}()
	waitContractSignal(t, joined, "pending-wake contenders")
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("submitPendingWakeup: %v", err)
		}
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("physical writes for %d successful contenders = %d, want 1", contenders, got)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
		t.Fatalf("pending state after successful contenders = %d, want pending", got)
	}
}

func TestFastWakeConsumptionPreservesPhysicalPendingEpoch(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}

	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseWrite) })
		closeFDResourcesT(t, loop)
	})
	var writes atomic.Int32
	loop.testHooks = &loopTestHooks{
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			writes.Add(1)
			close(writeEntered)
			<-releaseWrite
			return len(p), nil
		},
	}
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)

	submissionDone := make(chan struct{})
	go func() {
		loop.wakeAfterIngress()
		close(submissionDone)
	}()
	select {
	case <-writeEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("physical wake did not reach the descriptor write")
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalSubmitting {
		t.Fatalf("pending state during physical write = %d, want submitting", got)
	}

	// wakeAfterIngress publishes the fast signal before entering physical I/O.
	// Consuming that independent signal must not reopen the physical epoch.
	loop.pollFastMode(0)
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalSubmitting {
		t.Fatalf("pending state after fast wake consumption = %d, want submitting", got)
	}

	releaseOnce.Do(func() { close(releaseWrite) })
	select {
	case <-submissionDone:
	case <-time.After(5 * time.Second):
		t.Fatal("physical wake submission did not finish")
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
		t.Fatalf("pending state after physical write = %d, want pending", got)
	}

	loop.state.Store(StateSleeping)
	loop.wakeAfterIngress()
	if got := writes.Load(); got != 1 {
		t.Fatalf("physical writes in one undrained epoch = %d, want 1", got)
	}
}

func TestPhysicalWakeReleasesNativePollIO(t *testing.T) {
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	// Remove both RegisterFD notifications before observing the native poll.
	loop.drainWakeUpPipe()
	select {
	case <-loop.fastWakeupCh:
	default:
	}

	pollEntered := make(chan struct{})
	drainObserved := make(chan struct{})
	var pollOnce sync.Once
	var drainOnce sync.Once
	var writes atomic.Int32
	loop.testHooks = &loopTestHooks{
		PollIO: func(timeout int) (int, error) {
			pollOnce.Do(func() { close(pollEntered) })
			return loop.poller.PollIO(timeout)
		},
		WriteWakeFD: func(fd int, p []byte) (int, error) {
			writes.Add(1)
			return writeFD(fd, p)
		},
		AfterWakeDrain: func() { drainOnce.Do(func() { close(drainObserved) }) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	select {
	case <-pollEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not enter native PollIO")
	}

	executed := make(chan struct{})
	if err := loop.ScheduleMicrotask(func() { close(executed) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-drainObserved:
	case <-time.After(5 * time.Second):
		t.Fatal("native PollIO did not dispatch the physical wake descriptor")
	}
	select {
	case <-executed:
	case <-time.After(5 * time.Second):
		t.Fatal("microtask did not execute after native physical wake")
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("physical writes before completion = %d, want 1", got)
	}
}

func TestPendingWakeTreatsWrappedWouldBlockAsPhysicalPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)

	var writes atomic.Int32
	loop.testHooks = &loopTestHooks{
		WriteWakeFD: func(_ int, _ []byte) (int, error) {
			writes.Add(1)
			return 0, fmt.Errorf("wrapped: %w", unix.EWOULDBLOCK)
		},
	}
	if err := loop.submitPendingWakeup(); err != nil {
		t.Fatalf("submitPendingWakeup: %v", err)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
		t.Fatalf("pending state after would-block = %d, want pending", got)
	}
	if err := loop.submitPendingWakeup(); err != nil {
		t.Fatalf("deduplicated submitPendingWakeup: %v", err)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("physical writes in represented epoch = %d, want 1", got)
	}
}

func TestPendingWakeResourceReleaseWaitsForSubmission(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	loop.state.Store(StateAwake)
	loop.userIOFDCount.Store(1)

	writeEntered := make(chan struct{})
	closeReachedResource := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseWrite) })
		closeFDResourcesT(t, loop)
	})
	var writes atomic.Int32
	loop.testHooks = &loopTestHooks{
		BeforeWakeResourceClose: func() { close(closeReachedResource) },
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			if writes.Add(1) == 1 {
				close(writeEntered)
				<-releaseWrite
			}
			return len(p), nil
		},
	}

	wakeDone := make(chan error, 1)
	go func() { wakeDone <- loop.submitPendingWakeup() }()
	select {
	case <-writeEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("pending wake did not enter descriptor write")
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
		releaseOnce.Do(func() { close(releaseWrite) })
		if err := waitContractValue(t, wakeDone, "pending wake after unexpected resource-lock acquisition"); err != nil {
			t.Fatalf("pending wake after unexpected resource-lock acquisition: %v", err)
		}
		t.Fatal("pending physical submission did not retain wake-resource ownership")
	}
	releaseOnce.Do(func() { close(releaseWrite) })
	if err := waitContractValue(t, wakeDone, "pending physical wake submission"); err != nil {
		t.Fatalf("submitPendingWakeup: %v", err)
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish after pending wake submission")
	}
}

func TestIngressWakeFailureReleasesPendingClaim(t *testing.T) {
	sentinel := errors.New("wake write failed")
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	writes := 0
	loop.testHooks = &loopTestHooks{
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			writes++
			if writes == 1 {
				return 0, sentinel
			}
			return len(p), nil
		},
	}
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)

	loop.wakeAfterIngress()
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
		t.Fatalf("pending after failed ingress wake = %d, want idle", got)
	}
	loop.wakeAfterIngress()
	if writes != 2 {
		t.Fatalf("physical writes = %d, want retry after failed ingress submission", writes)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
		t.Fatalf("pending after successful ingress retry = %d, want pending", got)
	}
}

func TestSubmittingWakeLoserRetriesAfterWinnerFailure(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	firstWriteEntered := make(chan struct{})
	secondContending := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	var lockEntries atomic.Int32
	var writes atomic.Int32
	var unexpectedWrites atomic.Int32
	sentinel := errors.New("first wake write failed")
	loop.testHooks = &loopTestHooks{
		BeforePendingWakeLock: func() {
			if lockEntries.Add(1) == 2 {
				close(secondContending)
			}
		},
		WriteWakeFD: func(_ int, p []byte) (int, error) {
			switch writes.Add(1) {
			case 1:
				close(firstWriteEntered)
				<-releaseFirst
				return 0, sentinel
			case 2:
				return len(p), nil
			default:
				unexpectedWrites.Add(1)
				return 0, sentinel
			}
		},
	}
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)

	firstDone := make(chan struct{})
	go func() {
		loop.wakeAfterIngress()
		close(firstDone)
	}()
	select {
	case <-firstWriteEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first physical wake did not start")
	}
	secondDone := make(chan struct{})
	go func() {
		loop.wakeAfterIngress()
		close(secondDone)
	}()
	select {
	case <-secondContending:
	case <-time.After(5 * time.Second):
		t.Fatal("second wake did not contend with the submitting winner")
	}

	releaseOnce.Do(func() { close(releaseFirst) })
	for name, done := range map[string]<-chan struct{}{"first": firstDone, "second": secondDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s wake did not finish", name)
		}
	}
	if got := writes.Load(); got != 2 {
		t.Fatalf("physical writes = %d, want failed winner plus loser retry", got)
	}
	if got := unexpectedWrites.Load(); got != 0 {
		t.Fatalf("unexpected additional physical writes = %d", got)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
		t.Fatalf("pending state after loser retry = %d, want pending", got)
	}
}

func TestCompletedPendingWakeLoserAvoidsResourceLock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateSleeping)
	loop.userIOFDCount.Store(1)
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	var attemptedLock atomic.Bool
	loop.testHooks = &loopTestHooks{
		BeforePendingWakeLock: func() { attemptedLock.Store(true) },
	}
	loop.wakeMu.Lock()
	done := make(chan struct{})
	go func() {
		loop.wakeAfterIngress()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		loop.wakeMu.Unlock()
		t.Fatal("completed pending loser waited for wakeMu")
	}
	loop.wakeMu.Unlock()
	if attemptedLock.Load() {
		t.Fatal("completed pending loser entered the resource-lock path")
	}
}

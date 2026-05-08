package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLifecycle_LivenessAddingAPIsRejectDuringPublicTerminating(t *testing.T) {
	loop := New()

	stateTerminating := make(chan struct{})
	releaseShutdownHook := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(stateTerminating)
			<-releaseShutdownHook
		},
	}

	timerReady := make(chan TimerID, 1)
	releaseTask := make(chan struct{})
	if err := loop.Submit(func() {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			timerReady <- 0
			return
		}
		if err := loop.UnrefTimer(id); err != nil {
			timerReady <- 0
			return
		}
		timerReady <- id
		<-releaseTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	var existingTimer TimerID
	select {
	case existingTimer = <-timerReady:
		if existingTimer == 0 {
			close(releaseTask)
			t.Fatal("setup timer was not created and unref'd")
		}
	case <-time.After(5 * time.Second):
		close(releaseTask)
		t.Fatal("setup task did not create timer")
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	select {
	case <-stateTerminating:
	case <-time.After(5 * time.Second):
		close(releaseTask)
		t.Fatal("Shutdown did not enter StateTerminating")
	}

	if id, err := loop.ScheduleTimer(time.Hour, func() {}); err != ErrLoopTerminated || id != 0 {
		close(releaseShutdownHook)
		close(releaseTask)
		t.Fatalf("ScheduleTimer during StateTerminating = (%d, %v), want (0, ErrLoopTerminated)", id, err)
	}
	if err := loop.RefTimer(existingTimer); err != ErrLoopTerminated {
		close(releaseShutdownHook)
		close(releaseTask)
		t.Fatalf("RefTimer during StateTerminating = %v, want ErrLoopTerminated", err)
	}
	p := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		return "unexpected", nil
	})
	if p.State() != Rejected {
		close(releaseShutdownHook)
		close(releaseTask)
		t.Fatalf("Promisify during StateTerminating state = %v, want Rejected", p.State())
	}
	if reason, ok := p.Result().(error); !ok || !errors.Is(reason, ErrLoopTerminated) {
		close(releaseShutdownHook)
		close(releaseTask)
		t.Fatalf("Promisify during StateTerminating reason = %v, want ErrLoopTerminated", p.Result())
	}
	fd, cleanup := testCreateIOFD(t)
	defer cleanup()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != ErrLoopTerminated {
		close(releaseShutdownHook)
		close(releaseTask)
		t.Fatalf("RegisterFD during StateTerminating = %v, want ErrLoopTerminated", err)
	}

	close(releaseTask)
	close(releaseShutdownHook)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after public Shutdown")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}
}

func TestLifecycle_FDOperationsRejectAfterCloseCleanup(t *testing.T) {
	loop := New()

	fd, cleanup := testCreateIOFD(t)
	defer cleanup()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := loop.UnregisterFD(fd); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("UnregisterFD after Close = %v, want ErrLoopTerminated", err)
	}
	if err := loop.ModifyFD(fd, EventWrite); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("ModifyFD after Close = %v, want ErrLoopTerminated", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after Close cleanup = %d, want 0", got)
	}
}

func TestLifecycle_FDStateTerminatingAdmissionClasses(t *testing.T) {
	loop := New()
	defer closeFDResourcesT(t, loop)

	fd, cleanup := testCreateIOFD(t)
	defer cleanup()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	loop.state.Store(StateTerminating)
	if err := loop.ModifyFD(fd, EventWrite); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("ModifyFD during StateTerminating = %v, want ErrLoopTerminated", err)
	}
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("UnregisterFD during StateTerminating should remain valid cleanup, got %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after StateTerminating UnregisterFD = %d, want 0", got)
	}
}

func TestLifecycle_TerminateCleanupLinearizesConcurrentUnregister(t *testing.T) {
	loop := New()
	defer closeFDResourcesT(t, loop)

	fd, cleanup := testCreateIOFD(t)
	defer cleanup()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	loop.state.Store(StateTerminated)
	loop.livenessMu.Lock()
	cleanupDone := make(chan struct{})
	go func() {
		loop.terminateCleanup()
		close(cleanupDone)
	}()
	unregisterDone := make(chan error, 1)
	go func() { unregisterDone <- loop.UnregisterFD(fd) }()
	loop.livenessMu.Unlock()

	select {
	case <-cleanupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminateCleanup did not finish")
	}
	select {
	case err := <-unregisterDone:
		if !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("UnregisterFD racing terminated cleanup = %v, want ErrLoopTerminated", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UnregisterFD racing terminated cleanup did not finish")
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after terminated cleanup race = %d, want 0", got)
	}
}

func TestLifecycle_ScheduleTimerRejectsWhenShutdownWinsBeforeCommit(t *testing.T) {
	loop := New()

	stateTerminating := make(chan struct{})
	releaseShutdownHook := make(chan struct{})
	shutdownErr := make(chan error, 1)
	var hookOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(stateTerminating)
			<-releaseShutdownHook
		},
		BeforeScheduleTimerCommit: func() {
			hookOnce.Do(func() {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					shutdownErr <- loop.Shutdown(ctx)
				}()
				select {
				case <-stateTerminating:
				case <-time.After(5 * time.Second):
					t.Error("Shutdown did not reach StateTerminating before ScheduleTimer commit")
				}
			})
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if !errors.Is(err, ErrLoopTerminated) || id != 0 {
		close(releaseShutdownHook)
		t.Fatalf("ScheduleTimer after shutdown won transition = (%d, %v), want (0, ErrLoopTerminated)", id, err)
	}

	close(releaseShutdownHook)
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestLifecycle_LivenessCommitsBlockShutdownTransition(t *testing.T) {
	t.Run("RefTimer", func(t *testing.T) {
		loop := New()
		runDone := make(chan error, 1)
		shutdownStarted := make(chan struct{})
		shutdownErr := make(chan error, 1)
		var hookOnce sync.Once
		loop.testHooks = &loopTestHooks{
			BeforeTimerRefCommit: func() {
				hookOnce.Do(func() {
					go func() {
						close(shutdownStarted)
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						shutdownErr <- loop.Shutdown(ctx)
					}()
					<-shutdownStarted
				})
			},
		}
		go func() { runDone <- loop.Run(context.Background()) }()
		waitLoopOwnerTurnT(t, loop)
		if _, err := loop.ScheduleTimer(time.Hour, func() {}); err != nil {
			t.Fatalf("ScheduleTimer keepalive setup: %v", err)
		}
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer setup: %v", err)
		}
		if err := loop.UnrefTimer(id); err != nil {
			t.Fatalf("UnrefTimer setup: %v", err)
		}

		if err := loop.RefTimer(id); err != nil {
			t.Fatalf("RefTimer linearized before shutdown returned error: %v", err)
		}
		select {
		case err := <-shutdownErr:
			if err != nil {
				t.Fatalf("Shutdown returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Shutdown did not return")
		}
		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return")
		}
	})

	t.Run("RegisterFD", func(t *testing.T) {
		loop := New()
		runDone := make(chan error, 1)
		shutdownStarted := make(chan struct{})
		shutdownErr := make(chan error, 1)
		var hookOnce sync.Once
		loop.testHooks = &loopTestHooks{
			BeforeRegisterFDCommit: func() {
				hookOnce.Do(func() {
					go func() {
						close(shutdownStarted)
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						shutdownErr <- loop.Shutdown(ctx)
					}()
					<-shutdownStarted
				})
			},
		}
		go func() { runDone <- loop.Run(context.Background()) }()
		waitLoopOwnerTurnT(t, loop)
		if _, err := loop.ScheduleTimer(time.Hour, func() {}); err != nil {
			t.Fatalf("ScheduleTimer keepalive setup: %v", err)
		}
		fd, cleanup := testCreateIOFD(t)
		defer cleanup()
		if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
			t.Fatalf("RegisterFD linearized before shutdown returned error: %v", err)
		}
		select {
		case err := <-shutdownErr:
			if err != nil {
				t.Fatalf("Shutdown returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Shutdown did not return")
		}
		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return")
		}
	})

	t.Run("Promisify", func(t *testing.T) {
		loop := New()
		runDone := make(chan error, 1)
		shutdownStarted := make(chan struct{})
		shutdownErr := make(chan error, 1)
		workRelease := make(chan struct{})
		var hookOnce sync.Once
		loop.testHooks = &loopTestHooks{
			BeforePromisifyCommit: func() {
				hookOnce.Do(func() {
					go func() {
						close(shutdownStarted)
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						shutdownErr <- loop.Shutdown(ctx)
					}()
					<-shutdownStarted
				})
			},
		}
		go func() { runDone <- loop.Run(context.Background()) }()
		waitLoopOwnerTurnT(t, loop)
		if _, err := loop.ScheduleTimer(time.Hour, func() {}); err != nil {
			t.Fatalf("ScheduleTimer keepalive setup: %v", err)
		}
		p := loop.Promisify(context.Background(), func(context.Context) (any, error) {
			<-workRelease
			return "ok", nil
		})
		if resultErr, ok := p.Result().(error); p.State() == Rejected && ok && errors.Is(resultErr, ErrLoopTerminated) {
			close(workRelease)
			t.Fatal("Promisify was rejected even though it linearized before shutdown transition")
		}
		close(workRelease)
		select {
		case err := <-shutdownErr:
			if err != nil {
				t.Fatalf("Shutdown returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Shutdown did not return")
		}
		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return")
		}
		if state := p.State(); state != Fulfilled {
			t.Fatalf("Promisify state = %v, want Fulfilled", state)
		}
		if result := p.Result(); result != "ok" {
			t.Fatalf("Promisify result = %#v, want ok", result)
		}
	})
}

func TestCloseRunningReleasesPollerResources(t *testing.T) {
	if !fdPollingSupported {
		t.Skip("native poller resources are unavailable on task-only targets")
	}
	loop := New(WithFastPathMode(FastPathDisabled))
	registerLoopCleanupT(t, loop)
	if loop.pollerReady.Load() {
		t.Fatal("FastPathDisabled constructor eagerly initialized poller resources")
	}
	nativePoll := make(chan struct{}, 1)
	loop.testHooks = &loopTestHooks{BeforePollIO: func() {
		select {
		case nativePoll <- struct{}{}:
		default:
		}
	}}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, nativePoll, "FastPathDisabled native poll")
	if !loop.pollerReady.Load() {
		t.Fatal("native poll boundary did not initialize poller resources")
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after Close")
	}
	if loop.pollerReady.Load() {
		t.Fatal("poller resources remained ready after running Close")
	}
}
